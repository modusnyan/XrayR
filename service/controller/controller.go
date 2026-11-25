package controller

import (
	"errors"
	"fmt"
	"net"
	"os"
	"reflect"
	"runtime"
	"runtime/debug"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/xtls/xray-core/common/protocol"
	"github.com/xtls/xray-core/common/task"
	"github.com/xtls/xray-core/core"
	"github.com/xtls/xray-core/features/inbound"
	"github.com/xtls/xray-core/features/outbound"
	"github.com/xtls/xray-core/features/policy"
	"github.com/xtls/xray-core/features/stats"

	"github.com/XrayR-project/XrayR/api"
	"github.com/XrayR-project/XrayR/app/mydispatcher"
	"github.com/XrayR-project/XrayR/common/mylego"
	"github.com/XrayR-project/XrayR/common/serverstatus"
	"github.com/XrayR-project/XrayR/internal/snapshot"
	"github.com/XrayR-project/XrayR/observability"
	"github.com/XrayR-project/XrayR/service/diagnostics"
)

type LimitInfo struct {
	end               int64
	currentSpeedLimit int
	originSpeedLimit  uint64
}

// runtimeState is an immutable snapshot of the node/user/tag triple published by
// the node monitor. Readers load the whole snapshot atomically, so they never
// observe a partially updated state and never block the writer. Published values
// are never mutated in place; a new snapshot replaces the previous one.
type runtimeState struct {
	node  *api.NodeInfo
	users *[]api.UserInfo
	tag   string
}

type Controller struct {
	stateMu       sync.RWMutex
	closeOnce     sync.Once
	state         atomic.Pointer[runtimeState]
	server        *core.Instance
	config        *Config
	clientInfo    api.ClientInfo
	apiClient     api.Client
	tasks         []periodicTask
	limitedUsers  map[api.UserInfo]LimitInfo
	warnedUsers   map[api.UserInfo]int
	panelType     string
	capabilities  api.PanelCapabilities
	ibm           inbound.Manager
	obm           outbound.Manager
	stm           stats.Manager
	pm            policy.Manager
	dispatcher    *mydispatcher.DefaultDispatcher
	startAt       time.Time
	logger        *log.Entry
	snapshotStore *snapshot.Store
	lastSync      time.Time
	lastError     string
	// pullPeriod/pushPeriod are the effective sync intervals. pullPeriod drives
	// the node monitor and core heartbeat, pushPeriod drives the user monitor
	// (traffic reporting). Only the Start path and the node monitor write them,
	// and the node monitor writes them from inside the periodic task's own
	// goroutine, so no locking is needed: task.Periodic re-reads Interval when
	// it schedules each next run.
	pullPeriod time.Duration
	pushPeriod time.Duration
	// coreFailures counts consecutive failed liveness probes. Only the heartbeat
	// task writes it, but it is atomic so a future reader cannot race.
	coreFailures atomic.Int64
	// emptyUsersPending is set when the panel returned an empty user list while
	// the node still serves users. The list is only applied once the next pull
	// confirms it, so a single bad panel answer cannot disconnect every user.
	// Only the node monitor touches it; it is atomic for the same reason as above.
	emptyUsersPending atomic.Bool
	// nodeDisabled is set while the panel reports the node as switched off. The
	// inbound is removed and nothing is served until the panel enables the node
	// again, at which point the node monitor rebuilds it from a fresh config.
	nodeDisabled atomic.Bool
	// certWarnings holds the last set of certificate warnings, so each distinct
	// set is logged once (see checkCertConfig).
	certWarnings atomic.Value
	// configRejected is set while the panel's current node configuration is one
	// XrayR rejected. The panel then keeps answering 304 for it, so without this
	// flag the next round would report a clean sync and hide the problem. Only
	// a successfully applied configuration, or the panel sending one equal to
	// the active configuration, clears it. Written by the node monitor only.
	configRejected atomic.Bool
}

// runtimeSnapshot returns the current node, user list and tag. The returned
// values are immutable and must not be modified by callers.
func (c *Controller) runtimeSnapshot() (node *api.NodeInfo, users *[]api.UserInfo, tag string) {
	if snapshot := c.state.Load(); snapshot != nil {
		return snapshot.node, snapshot.users, snapshot.tag
	}
	return nil, nil, ""
}

// Tag returns the inbound tag currently in use. Safe for concurrent use.
func (c *Controller) Tag() string {
	_, _, tag := c.runtimeSnapshot()
	return tag
}

func (c *Controller) currentNode() *api.NodeInfo {
	node, _, _ := c.runtimeSnapshot()
	return node
}

func (c *Controller) currentUsers() *[]api.UserInfo {
	_, users, _ := c.runtimeSnapshot()
	return users
}

// publishState atomically replaces the node/user/tag triple.
func (c *Controller) publishState(node *api.NodeInfo, users *[]api.UserInfo, tag string) {
	c.state.Store(&runtimeState{node: node, users: users, tag: tag})
}

// publishUsers atomically replaces the user list, keeping node and tag unchanged.
func (c *Controller) publishUsers(users *[]api.UserInfo) {
	node, _, tag := c.runtimeSnapshot()
	c.state.Store(&runtimeState{node: node, users: users, tag: tag})
}

type periodicTask struct {
	tag string
	*task.Periodic
}

// New return a Controller service with default parameters.
func New(server *core.Instance, client api.Client, config *Config, panelType string, capabilities api.PanelCapabilities) *Controller {
	clientInfo := client.Describe()
	logger := log.NewEntry(log.StandardLogger()).WithFields(log.Fields{
		"Host": clientInfo.APIHost,
		"Type": clientInfo.NodeType,
		"ID":   clientInfo.NodeID,
	})
	controller := &Controller{
		server:       server,
		config:       config,
		apiClient:    client,
		panelType:    panelType,
		capabilities: capabilities,
		ibm:          server.GetFeature(inbound.ManagerType()).(inbound.Manager),
		obm:          server.GetFeature(outbound.ManagerType()).(outbound.Manager),
		stm:          server.GetFeature(stats.ManagerType()).(stats.Manager),
		pm:           server.GetFeature(policy.ManagerType()).(policy.Manager),
		dispatcher:   server.GetFeature(mydispatcher.Type()).(*mydispatcher.DefaultDispatcher),
		startAt:      time.Now(),
		logger:       logger,
		// Defaults for paths that run before Start() applies the panel's
		// intervals (e.g. tests invoking a monitor directly).
		pullPeriod: time.Duration(config.UpdatePeriodic) * time.Second,
		pushPeriod: time.Duration(config.UpdatePeriodic) * time.Second,
	}
	if config.SnapshotPath != "" {
		controller.snapshotStore = snapshot.New(config.SnapshotPath, clientInfo, time.Duration(config.SnapshotMaxAge)*time.Second)
	}

	return controller
}

// Start implement the Start() function of the service interface
func (c *Controller) Start() error {
	c.clientInfo = c.apiClient.Describe()
	newNodeInfo, nodeErr := c.apiClient.GetNodeInfo()
	var userInfo *[]api.UserInfo
	var rules *[]api.DetectRule
	if nodeErr == nil {
		userInfo, nodeErr = c.apiClient.GetUserList()
	}
	// A node the panel switched off must not come back from the snapshot. Start
	// it offline instead: the monitors keep polling and bring it up once the
	// panel enables it again.
	if errors.Is(nodeErr, api.ErrNodeDisabled) {
		return c.startDisabled()
	}
	if nodeErr == nil && !c.config.DisableGetRule {
		if provider, ok := c.apiClient.(api.RuleProvider); ok {
			rules, _ = provider.GetNodeRule()
		}
	}
	if nodeErr != nil && c.snapshotStore != nil {
		var savedAt time.Time
		newNodeInfo, userInfo, rules, savedAt, nodeErr = c.snapshotStore.Load()
		if nodeErr == nil {
			c.logger.WithField("saved_at", savedAt).Warn("Panel is unavailable; using the last valid snapshot")
		} else if os.IsNotExist(nodeErr) {
			c.logger.Warn("Panel is unavailable and no snapshot exists; ignoring snapshot error")
			nodeErr = nil
		}
	}
	if nodeErr != nil {
		return nodeErr
	}
	if err := validateRuntimeSnapshot(newNodeInfo, userInfo); err != nil {
		return err
	}
	nodeTag := c.buildNodeTag(newNodeInfo)
	c.publishState(newNodeInfo, userInfo, nodeTag)

	if err := c.addNewTag(newNodeInfo, nodeTag); err != nil {
		return err
	}
	if err := c.addNewUser(userInfo, newNodeInfo, nodeTag); err != nil {
		_ = c.removeOldTag(nodeTag)
		return err
	}
	if err := c.AddInboundLimiter(nodeTag, newNodeInfo.SpeedLimit, userInfo, c.config.GlobalDeviceLimitConfig); err != nil {
		c.logger.Print(err)
	}
	if rules != nil && len(*rules) > 0 {
		if err := c.UpdateRule(nodeTag, *rules); err != nil {
			c.logger.Print(err)
		}
	}
	if c.snapshotStore != nil {
		if err := c.snapshotStore.Save(newNodeInfo, userInfo, rules); err != nil {
			c.logger.WithError(err).Warn("Failed to save runtime snapshot")
		}
	}

	c.checkCertConfig()
	c.startMonitors(newNodeInfo.EnableTLS && !c.config.EnableREALITY)
	c.stateMu.Lock()
	c.lastSync = time.Now()
	c.lastError = ""
	c.stateMu.Unlock()
	observability.LastSync.WithLabelValues(c.panelType, newNodeInfo.NodeType).Set(float64(time.Now().Unix()))
	observability.Users.WithLabelValues(c.panelType, newNodeInfo.NodeType).Set(float64(len(*userInfo)))
	return nil
}

// startDisabled starts the controller without an inbound because the panel
// reports the node as disabled.
func (c *Controller) startDisabled() error {
	c.logger.Warn("Panel reports this node as disabled; it stays offline until the panel enables it")
	c.nodeDisabled.Store(true)
	c.publishState(nil, nil, "")
	c.removeSnapshot()
	// The node may be enabled later with TLS, so register the cert monitor; it
	// is a no-op while there is no TLS node.
	c.startMonitors(!c.config.EnableREALITY)
	c.recordNodeDisabled()
	return nil
}

// startMonitors initialises the monitor state and launches the periodic tasks.
func (c *Controller) startMonitors(withCertMonitor bool) {
	if c.config.AutoSpeedLimitConfig == nil {
		c.config.AutoSpeedLimitConfig = &AutoSpeedLimitConfig{}
	}
	if c.config.AutoSpeedLimitConfig.Limit > 0 {
		c.limitedUsers = make(map[api.UserInfo]LimitInfo)
		c.warnedUsers = make(map[api.UserInfo]int)
	}
	// The panel may steer the sync cadence (Xboard base_config.push_interval /
	// pull_interval). Pull drives the node monitor and core heartbeat, push
	// drives the user monitor (traffic reporting); values <= 0 keep the locally
	// configured period.
	c.pullPeriod = time.Duration(c.config.UpdatePeriodic) * time.Second
	c.pushPeriod = c.pullPeriod
	if provider, ok := c.apiClient.(api.IntervalsProvider); ok {
		if provider.PullInterval() > 0 {
			c.pullPeriod = time.Duration(provider.PullInterval()) * time.Second
		}
		if provider.PushInterval() > 0 {
			c.pushPeriod = time.Duration(provider.PushInterval()) * time.Second
		}
		if c.pullPeriod != c.pushPeriod {
			c.logger.Printf("Panel intervals applied: pull=%s push=%s", c.pullPeriod, c.pushPeriod)
		}
	}
	c.tasks = append(c.tasks,
		periodicTask{tag: "node monitor", Periodic: &task.Periodic{Interval: c.pullPeriod, Execute: c.guardTask("node monitor", c.nodeInfoMonitor)}},
		periodicTask{tag: "user monitor", Periodic: &task.Periodic{Interval: c.pushPeriod, Execute: c.guardTask("user monitor", c.userInfoMonitor)}},
		periodicTask{tag: "core heartbeat", Periodic: &task.Periodic{Interval: c.pullPeriod, Execute: c.guardTask("core heartbeat", c.coreHeartbeatMonitor)}},
	)
	if withCertMonitor {
		c.tasks = append(c.tasks, periodicTask{tag: "cert monitor", Periodic: &task.Periodic{Interval: time.Duration(c.config.UpdatePeriodic) * time.Minute, Execute: c.guardTask("cert monitor", c.certMonitor)}})
	}
	for i := range c.tasks {
		c.logger.Printf("Start %s periodic task", c.tasks[i].tag)
		go func(periodic *task.Periodic, name string) {
			// task.Periodic stops for good when Execute returns an error, so surface
			// the reason instead of letting the task disappear silently.
			if err := periodic.Start(); err != nil {
				c.logger.WithError(err).Errorf("%s periodic task stopped", name)
			}
		}(c.tasks[i].Periodic, c.tasks[i].tag)
	}
}

// guardTask contains panics raised by a periodic task body.
//
// xray-core's task.Periodic neither recovers from panics nor survives a non-nil
// error (it clears its running flag), so an unguarded panic inside a monitor would
// take down the whole node process. The recovered panic is logged with a stack
// trace and swallowed so the task keeps running on its next tick.
func (c *Controller) guardTask(name string, fn func() error) func() error {
	return func() (err error) {
		defer func() {
			if recovered := recover(); recovered != nil {
				c.logger.WithFields(log.Fields{
					"task":  name,
					"panic": recovered,
					"stack": string(debug.Stack()),
				}).Error("Recovered from panic in periodic task")
				err = nil
			}
		}()
		return fn()
	}
}

func validateRuntimeSnapshot(node *api.NodeInfo, users *[]api.UserInfo) error {
	if node == nil || node.Port == 0 {
		return errors.New("server port must be greater than zero")
	}
	// An empty list is valid (the node serves nobody); a nil one means no list
	// was obtained at all.
	if users == nil {
		return errors.New("user list is missing")
	}
	return nil
}

// confirmUserList decides which user list the node monitor applies.
//
// fetched is the list the panel just returned (nil when notModified is set).
// An empty list while the node still serves users is held back once: the node
// keeps its current users and the next pull must confirm the empty list,
// either with another empty 200 or with a 304 against the empty list's ETag.
// Any non-empty answer cancels the pending confirmation.
func (c *Controller) confirmUserList(fetched *[]api.UserInfo, notModified bool) (users *[]api.UserInfo, changed bool) {
	current := c.currentUsers()
	currentCount := 0
	if current != nil {
		currentCount = len(*current)
	}

	if notModified {
		if c.emptyUsersPending.CompareAndSwap(true, false) {
			// The cached ETag belongs to the empty list held back last round.
			c.logger.Warnf("Panel confirmed an empty user list; removing %d users", currentCount)
			empty := []api.UserInfo{}
			return &empty, true
		}
		return current, false
	}

	if fetched != nil && len(*fetched) == 0 && currentCount > 0 {
		if !c.emptyUsersPending.Load() {
			c.emptyUsersPending.Store(true)
			c.logger.Warnf("Panel returned an empty user list; keeping %d users until the next pull confirms it", currentCount)
			return current, false
		}
		c.emptyUsersPending.Store(false)
		c.logger.Warnf("Panel confirmed an empty user list; removing %d users", currentCount)
		return fetched, true
	}

	c.emptyUsersPending.Store(false)
	return fetched, true
}

// Close implement the Close() function of the service interface
func (c *Controller) Close() error {
	var closeErr error
	c.closeOnce.Do(func() {
		for i := range c.tasks {
			if c.tasks[i].Periodic != nil {
				if err := c.tasks[i].Close(); err != nil && closeErr == nil {
					closeErr = fmt.Errorf("%s periodic task close failed: %w", c.tasks[i].tag, err)
				}
			}
		}
		if closer, ok := c.apiClient.(api.Closer); ok {
			if err := closer.Close(); err != nil && closeErr == nil {
				closeErr = err
			}
		}
	})
	return closeErr
}

// DiagnosticStatus returns sanitized, low-cardinality runtime state.
func (c *Controller) DiagnosticStatus() diagnostics.NodeStatus {
	node, users, tag := c.runtimeSnapshot()
	c.stateMu.RLock()
	lastSync, lastError := c.lastSync, c.lastError
	c.stateMu.RUnlock()
	userCount := 0
	if users != nil {
		userCount = len(*users)
	}
	return diagnostics.NodeStatus{
		Panel: c.panelType, NodeID: c.clientInfo.NodeID, NodeType: c.clientInfo.NodeType,
		Ready: node != nil && tag != "", Users: userCount, LastSync: lastSync, LastError: lastError,
	}
}

// intervalOrFallback returns seconds when it is positive, otherwise fallback.
func intervalOrFallback(seconds, fallback int) int {
	if seconds > 0 {
		return seconds
	}
	return fallback
}

func (c *Controller) nodeInfoMonitor() (err error) {
	// delay to start
	if time.Since(c.startAt) < c.pullPeriod {
		return nil
	}

	// First fetch Node Info
	var nodeInfoChanged = true
	newNodeInfo, err := c.apiClient.GetNodeInfo()
	if err != nil {
		if errors.Is(err, api.ErrNodeNotModified) {
			nodeInfoChanged = false
			newNodeInfo = c.currentNode()
		} else if errors.Is(err, api.ErrNodeDisabled) {
			c.takeNodeDown()
			return nil
		} else {
			// A transport, HTTP or JSON failure is an *api.APIError and passes
			// with the next good round. Anything else is a configuration XrayR
			// cannot serve (e.g. an unsupported Shadowsocks plugin) and stays
			// reported until the panel changes it.
			var apiErr *api.APIError
			if errors.As(err, &apiErr) {
				c.recordSyncError(err)
			} else {
				c.recordRejectedConfig(err)
			}
			return nil
		}
	}
	if newNodeInfo == nil {
		// 304 while the node is offline: there is no configuration to apply.
		return nil
	}
	if newNodeInfo.Port == 0 {
		return errors.New("server port must > 0")
	}

	// Follow the cadence the panel advertises, if any. Writing the fields here
	// is safe: this runs on the node monitor's own periodic goroutine and
	// task.Periodic re-reads Interval right after Execute returns.
	if provider, ok := c.apiClient.(api.IntervalsProvider); ok {
		newPull := time.Duration(intervalOrFallback(provider.PullInterval(), c.config.UpdatePeriodic)) * time.Second
		newPush := time.Duration(intervalOrFallback(provider.PushInterval(), c.config.UpdatePeriodic)) * time.Second
		if newPull != c.pullPeriod {
			c.pullPeriod = newPull
			c.logger.Printf("Panel pull interval changed to %s", newPull)
		}
		if newPush != c.pushPeriod {
			c.pushPeriod = newPush
			c.logger.Printf("Panel push interval changed to %s", newPush)
		}
		for i := range c.tasks {
			switch c.tasks[i].tag {
			case "node monitor", "core heartbeat":
				c.tasks[i].Interval = c.pullPeriod
			case "user monitor":
				c.tasks[i].Interval = c.pushPeriod
			}
		}
	}

	// Update User
	newUserInfo, err := c.apiClient.GetUserList()
	notModified := errors.Is(err, api.ErrUserNotModified)
	if errors.Is(err, api.ErrNodeDisabled) {
		c.takeNodeDown()
		return nil
	}
	if err != nil && !notModified {
		c.logger.Print(err)
		return nil
	}
	if err != nil {
		// ErrUserNotModified: no fresh list in this response.
		newUserInfo = nil
	}
	var usersChanged bool
	if c.nodeDisabled.Load() {
		// Coming back online: apply the panel's list as is. There are no users
		// being served whose removal would need confirming.
		if newUserInfo == nil {
			return nil
		}
		usersChanged = true
	} else {
		newUserInfo, usersChanged = c.confirmUserList(newUserInfo, notModified)
	}

	// Apply a node change transactionally. Validate the complete replacement
	// before removing the active tag, then rebuild the previous snapshot if any
	// runtime operation fails.
	if nodeInfoChanged {
		activeNode := c.currentNode()
		if !reflect.DeepEqual(activeNode, newNodeInfo) {
			if err := validateRuntimeSnapshot(newNodeInfo, newUserInfo); err != nil {
				c.recordRejectedConfig(err)
				return nil
			}
			newTag := fmt.Sprintf("%s_%s_%d", newNodeInfo.NodeType, c.config.ListenIP, newNodeInfo.Port)
			if _, err := InboundBuilder(c.config, newNodeInfo, newTag); err != nil {
				c.recordRejectedConfig(err)
				return nil
			}
			if _, err := OutboundBuilder(c.config, newNodeInfo, newTag); err != nil {
				c.recordRejectedConfig(err)
				return nil
			}

			oldNode, oldUsers, oldTag := c.runtimeSnapshot()
			// A disabled node has no inbound left to remove or roll back to.
			wasDisabled := c.nodeDisabled.Load()
			if !wasDisabled {
				if err := c.removeOldTag(oldTag); err != nil {
					c.recordSyncError(err)
					return nil
				}
				_ = c.DeleteInboundLimiter(oldTag)
			}
			c.publishState(newNodeInfo, newUserInfo, newTag)
			applyErr := c.addNewTag(newNodeInfo, newTag)
			if applyErr == nil {
				applyErr = c.addNewUser(newUserInfo, newNodeInfo, newTag)
			}
			if applyErr == nil {
				applyErr = c.AddInboundLimiter(newTag, newNodeInfo.SpeedLimit, newUserInfo, c.config.GlobalDeviceLimitConfig)
			}
			if applyErr != nil {
				_ = c.removeOldTag(newTag)
				_ = c.DeleteInboundLimiter(newTag)
				if wasDisabled {
					c.publishState(nil, nil, "")
					c.recordRejectedConfig(fmt.Errorf("panel re-enabled the node but its configuration was rejected; the node stays offline: %w", applyErr))
					return nil
				}
				c.publishState(oldNode, oldUsers, oldTag)
				rollbackErr := c.addNewTag(oldNode, oldTag)
				if rollbackErr == nil {
					rollbackErr = c.addNewUser(oldUsers, oldNode, oldTag)
				}
				if rollbackErr == nil {
					rollbackErr = c.AddInboundLimiter(oldTag, oldNode.SpeedLimit, oldUsers, c.config.GlobalDeviceLimitConfig)
				}
				if rollbackErr != nil {
					c.recordRejectedConfig(fmt.Errorf("apply failed: %v; rollback failed: %w", applyErr, rollbackErr))
				} else {
					c.recordRejectedConfig(fmt.Errorf("new node configuration rejected and rolled back: %w", applyErr))
				}
				return nil
			}
			if wasDisabled {
				c.nodeDisabled.Store(false)
				c.logger.Warnf("Panel re-enabled the node; serving %d users again", len(*newUserInfo))
			}
			c.configRejected.Store(false)
			c.markSynced()
			nodeInfoChanged = true
		} else {
			// The panel is back on the configuration that is already running.
			c.configRejected.Store(false)
			nodeInfoChanged = false
		}
	}

	// Check Rule
	if !c.config.DisableGetRule && c.capabilities.Rules {
		if provider, ok := c.apiClient.(api.RuleProvider); ok {
			if ruleList, err := provider.GetNodeRule(); err != nil {
				if !errors.Is(err, api.ErrRuleNotModified) {
					c.logger.Printf("Get rule list failed: %s", err)
				}
			} else if len(*ruleList) > 0 {
				if err := c.UpdateRule(c.Tag(), *ruleList); err != nil {
					c.logger.Print(err)
				}
			}
		}
	}

	if !nodeInfoChanged {
		tag := c.Tag()
		var deleted, added []api.UserInfo
		if usersChanged {
			deleted, added = compareUserList(c.currentUsers(), newUserInfo)
			if len(deleted) > 0 {
				deletedEmail := make([]string, len(deleted))
				for i, u := range deleted {
					deletedEmail[i] = fmt.Sprintf("%s|%s|%d", tag, u.Email, u.UID)
				}
				err := c.removeUsers(deletedEmail, tag)
				if err != nil {
					c.logger.Print(err)
				}
			}
			if len(added) > 0 {
				err = c.addNewUser(&added, c.currentNode(), tag)
				if err != nil {
					c.logger.Print(err)
				}
				// Update Limiter
				if err := c.UpdateInboundLimiter(tag, &added); err != nil {
					c.logger.Print(err)
				}
			}
		}
		c.logger.Printf("%d user deleted, %d user added", len(deleted), len(added))
	}
	c.publishUsers(newUserInfo)
	if c.snapshotStore != nil {
		var rules *[]api.DetectRule
		if !c.config.DisableGetRule {
			if provider, ok := c.apiClient.(api.RuleProvider); ok {
				rules, _ = provider.GetNodeRule()
			}
		}
		node, users, _ := c.runtimeSnapshot()
		if err := c.snapshotStore.Save(node, users, rules); err != nil {
			c.logger.WithError(err).Warn("Failed to update runtime snapshot")
		}
	}
	c.checkCertConfig()
	c.markSynced()
	return nil
}

// markSynced records a successful synchronization and refreshes the gauges.
func (c *Controller) markSynced() {
	node, users, _ := c.runtimeSnapshot()
	c.stateMu.Lock()
	c.lastSync = time.Now()
	if !c.configRejected.Load() {
		c.lastError = ""
	}
	c.stateMu.Unlock()
	if node == nil {
		return
	}
	observability.LastSync.WithLabelValues(c.panelType, node.NodeType).Set(float64(time.Now().Unix()))
	if users != nil {
		observability.Users.WithLabelValues(c.panelType, node.NodeType).Set(float64(len(*users)))
	}
}

func (c *Controller) recordSyncError(err error) {
	c.stateMu.Lock()
	c.lastError = err.Error()
	c.stateMu.Unlock()
	c.logger.WithError(err).Warn("Node synchronization failed; keeping the last valid runtime snapshot")
}

// recordRejectedConfig records that the panel's current node configuration was
// rejected; the error stays reported until the panel sends a different one.
func (c *Controller) recordRejectedConfig(err error) {
	c.configRejected.Store(true)
	c.recordSyncError(err)
}

// nodeStatus collects what the node reports to the panel each push period.
func (c *Controller) nodeStatus(users *[]api.UserInfo) *api.NodeStatus {
	status := &api.NodeStatus{Goroutines: runtime.NumGoroutine()}
	system, err := serverstatus.GetSystemStatus()
	if err != nil {
		// Containers often hide part of /proc; report what could be read.
		c.logger.WithError(err).Debug("System status is incomplete")
	}
	status.CPU, status.Uptime = system.CPU, system.Uptime
	status.MemTotal, status.MemUsed = system.MemTotal, system.MemUsed
	status.SwapTotal, status.SwapUsed = system.SwapTotal, system.SwapUsed
	status.DiskTotal, status.DiskUsed = system.DiskTotal, system.DiskUsed
	if system.MemTotal > 0 {
		status.Mem = float64(system.MemUsed) / float64(system.MemTotal) * 100
	}
	if system.DiskTotal > 0 {
		status.Disk = float64(system.DiskUsed) / float64(system.DiskTotal) * 100
	}
	if users != nil {
		status.Users = len(*users)
	}
	c.stateMu.RLock()
	status.SyncError = c.lastError
	if !c.lastSync.IsZero() {
		status.LastSyncAt = c.lastSync.Unix()
	}
	c.stateMu.RUnlock()
	return status
}

// recordNodeDisabled surfaces the disabled state in diagnostics.
func (c *Controller) recordNodeDisabled() {
	c.stateMu.Lock()
	c.lastError = api.ErrNodeDisabled.Error()
	c.stateMu.Unlock()
}

// removeSnapshot deletes the runtime snapshot, if snapshots are enabled.
func (c *Controller) removeSnapshot() {
	if c.snapshotStore == nil {
		return
	}
	if err := c.snapshotStore.Remove(); err != nil {
		c.logger.WithError(err).Warn("Failed to remove the runtime snapshot of the disabled node")
	}
}

// takeNodeDown stops serving after the panel reported the node as disabled.
//
// The inbound and its limiter are removed at once, so no new connection is
// accepted, and the snapshot is deleted so a restart during a panel outage
// cannot bring the node back. The user list and tag stay in the runtime state
// (with the node cleared) until the user monitor has pushed the traffic counted
// before the shutdown; the panel still accepts push for a disabled node.
// Runs on the node monitor's goroutine; later calls while offline are no-ops.
func (c *Controller) takeNodeDown() {
	if c.nodeDisabled.Load() {
		return
	}
	_, users, tag := c.runtimeSnapshot()
	if tag != "" {
		if err := c.removeOldTag(tag); err != nil {
			c.logger.WithError(err).Error("Failed to remove the inbound of the disabled node")
		}
		_ = c.DeleteInboundLimiter(tag)
	}
	c.publishState(nil, users, tag)
	c.nodeDisabled.Store(true)
	c.emptyUsersPending.Store(false)
	c.removeSnapshot()
	c.recordNodeDisabled()
	count := 0
	if users != nil {
		count = len(*users)
	}
	c.logger.Warnf("Panel disabled the node; stopped serving %d users", count)
}

func (c *Controller) removeOldTag(oldTag string) (err error) {
	err = c.removeInbound(oldTag)
	if err != nil {
		return err
	}
	err = c.removeOutbound(oldTag)
	if err != nil {
		return err
	}
	return nil
}

func (c *Controller) addNewTag(newNodeInfo *api.NodeInfo, tag string) (err error) {
	inboundConfig, err := InboundBuilder(c.config, newNodeInfo, tag)
	if err != nil {
		return err
	}
	if err = c.addInbound(inboundConfig); err != nil {
		return err
	}
	outBoundConfig, err := OutboundBuilder(c.config, newNodeInfo, tag)
	if err != nil {
		return err
	}
	return c.addOutbound(outBoundConfig)
}

func (c *Controller) addNewUser(userInfo *[]api.UserInfo, nodeInfo *api.NodeInfo, tag string) (err error) {
	var users []*protocol.User
	switch nodeInfo.NodeType {
	case "V2ray", "Vmess", "Vless":
		if nodeInfo.EnableVless || (nodeInfo.NodeType == "Vless" && nodeInfo.NodeType != "Vmess") {
			users = c.buildVlessUser(userInfo, tag, nodeInfo.VlessFlow)
		} else {
			users = c.buildVmessUser(userInfo, tag)
		}
	case "Trojan":
		users = c.buildTrojanUser(userInfo, tag)
	case "Shadowsocks":
		users = c.buildSSUser(userInfo, nodeInfo.CypherMethod, tag)
	default:
		return fmt.Errorf("unsupported node type: %s", nodeInfo.NodeType)
	}

	err = c.addUsers(users, tag)
	if err != nil {
		return err
	}
	c.logger.Printf("Added %d new users", len(*userInfo))
	return nil
}

func compareUserList(old, new *[]api.UserInfo) (deleted, added []api.UserInfo) {
	mSrc := make(map[api.UserInfo]byte) // 按源数组建索引
	mAll := make(map[api.UserInfo]byte) // 源+目所有元素建索引

	var set []api.UserInfo // 交集

	// 1.源数组建立map
	for _, v := range *old {
		mSrc[v] = 0
		mAll[v] = 0
	}
	// 2.目数组中，存不进去，即重复元素，所有存不进去的集合就是并集
	for _, v := range *new {
		before := len(mAll)
		mAll[v] = 1
		if before == len(mAll) { // 长度没变，说明该元素已存在，属于交集
			set = append(set, v)
		}
	}
	// 3.遍历交集，在并集中找，找到就从并集中删，删完后就是补集（即并-交=所有变化的元素）
	for _, v := range set {
		delete(mAll, v)
	}
	// 4.此时，mall是补集，所有元素去源中找，找到就是删除的，找不到的必定能在目数组中找到，即新加的
	for v := range mAll {
		_, exist := mSrc[v]
		if exist {
			deleted = append(deleted, v)
		} else {
			added = append(added, v)
		}
	}

	return deleted, added
}

func limitUser(c *Controller, tag string, user api.UserInfo, silentUsers *[]api.UserInfo) {
	c.limitedUsers[user] = LimitInfo{
		end:               time.Now().Unix() + int64(c.config.AutoSpeedLimitConfig.LimitDuration*60),
		currentSpeedLimit: c.config.AutoSpeedLimitConfig.LimitSpeed,
		originSpeedLimit:  user.SpeedLimit,
	}
	c.logger.Printf("Limit User: %s Speed: %d End: %s", buildUserTag(tag, &user), c.config.AutoSpeedLimitConfig.LimitSpeed, time.Unix(c.limitedUsers[user].end, 0).Format("01-02 15:04:05"))
	user.SpeedLimit = uint64((c.config.AutoSpeedLimitConfig.LimitSpeed * 1000000) / 8)
	*silentUsers = append(*silentUsers, user)
}

func (c *Controller) userInfoMonitor() (err error) {
	// Delay to start. The user monitor shares startAt with the other monitors
	// and may run on a different (push) period, so gate on the pull period to
	// keep the first round aligned with the initial sync.
	if time.Since(c.startAt) < c.pullPeriod {
		return nil
	}

	// Read the runtime state once. The monitors share these values with the node
	// monitor, which may replace them at any time.
	state := c.state.Load()
	var tag string
	var userList *[]api.UserInfo
	if state != nil {
		tag, userList = state.tag, state.users
	}

	// A disabled node has nothing to report, and the panel refuses everything
	// but traffic pushes for it.
	nodeDisabled := c.nodeDisabled.Load()

	if c.capabilities.NodeStatusReport && !nodeDisabled {
		if reporter, ok := c.apiClient.(api.NodeStatusReporter); ok {
			if statusErr := reporter.ReportNodeStatus(c.nodeStatus(userList)); statusErr != nil {
				c.logger.Print(statusErr)
			}
		}
	}

	if c.config.AutoSpeedLimitConfig.Limit > 0 && len(c.limitedUsers) > 0 {
		c.logger.Printf("Limited users:")
		toReleaseUsers := make([]api.UserInfo, 0)
		for user, limitInfo := range c.limitedUsers {
			if time.Now().Unix() > limitInfo.end {
				user.SpeedLimit = limitInfo.originSpeedLimit
				toReleaseUsers = append(toReleaseUsers, user)
				c.logger.Printf("User: %s Speed: %d End: nil (Unlimit)", buildUserTag(tag, &user), user.SpeedLimit)
				delete(c.limitedUsers, user)
			} else {
				c.logger.Printf("User: %s Speed: %d End: %s", buildUserTag(tag, &user), limitInfo.currentSpeedLimit, time.Unix(c.limitedUsers[user].end, 0).Format("01-02 15:04:05"))
			}
		}
		if len(toReleaseUsers) > 0 {
			if err := c.UpdateInboundLimiter(tag, &toReleaseUsers); err != nil {
				c.logger.Print(err)
			}
		}
	}

	var userTraffic []api.UserTraffic
	var upCounterList []stats.Counter
	var downCounterList []stats.Counter
	// AutoSpeedLimit thresholds are expressed per reporting period, which is the
	// effective push period rather than the static config value.
	AutoSpeedLimit := int64(c.config.AutoSpeedLimitConfig.Limit)
	UpdatePeriodic := int64(c.pushPeriod / time.Second)
	if UpdatePeriodic <= 0 {
		UpdatePeriodic = int64(c.config.UpdatePeriodic)
	}
	limitedUsers := make([]api.UserInfo, 0)
	if userList != nil {
		for _, user := range *userList {
			userTag := buildUserTag(tag, &user)
			up, down, upCounter, downCounter := c.getTraffic(userTag)
			if down > 0 {
				c.logger.Printf("Traffic counted: tag=%s up=%d down=%d", userTag, up, down)
			}
			if up > 0 || down > 0 {
				if AutoSpeedLimit > 0 {
					if down > AutoSpeedLimit*1000000*UpdatePeriodic/8 || up > AutoSpeedLimit*1000000*UpdatePeriodic/8 {
						if _, ok := c.limitedUsers[user]; !ok {
							if c.config.AutoSpeedLimitConfig.WarnTimes == 0 {
								limitUser(c, tag, user, &limitedUsers)
							} else {
								c.warnedUsers[user]++
								if c.warnedUsers[user] > c.config.AutoSpeedLimitConfig.WarnTimes {
									limitUser(c, tag, user, &limitedUsers)
									delete(c.warnedUsers, user)
								}
							}
						}
					} else {
						delete(c.warnedUsers, user)
					}
				}
				userTraffic = append(userTraffic, api.UserTraffic{UID: user.UID, Email: user.Email, Upload: up, Download: down})
				if upCounter != nil {
					upCounterList = append(upCounterList, upCounter)
				}
				if downCounter != nil {
					downCounterList = append(downCounterList, downCounter)
				}
			} else {
				delete(c.warnedUsers, user)
			}
		}
	}
	if len(limitedUsers) > 0 {
		if err := c.UpdateInboundLimiter(tag, &limitedUsers); err != nil {
			c.logger.Print(err)
		}
	}
	trafficReported := true
	if len(userTraffic) > 0 {
		c.logger.Printf("Reporting %d user(s) traffic to panel", len(userTraffic))
		var reportErr error
		if c.capabilities.TrafficReport {
			if reporter, ok := c.apiClient.(api.TrafficReporter); ok {
				if !c.config.DisableUploadTraffic {
					reportErr = reporter.ReportUserTraffic(&userTraffic)
				}
			} else if !c.config.DisableUploadTraffic {
				reportErr = errors.New("panel does not implement its declared traffic reporting capability")
			}
		} else if !c.config.DisableUploadTraffic {
			reportErr = errors.New("panel does not support traffic reporting")
		}
		if reportErr != nil {
			observability.TrafficFailures.WithLabelValues(c.panelType).Inc()
			c.logger.Print(reportErr)
			trafficReported = false
		} else {
			c.resetTraffic(&upCounterList, &downCounterList)
		}
	}

	// After the panel disabled the node the state keeps its users only until
	// their remaining traffic is pushed. Drop them now, unless the node monitor
	// published a new state in the meantime (the swap then fails harmlessly).
	if state != nil && state.node == nil && state.users != nil && trafficReported {
		if c.state.CompareAndSwap(state, &runtimeState{}) {
			c.logger.Printf("Reported the remaining traffic of the disabled node; cleared %d users", len(*state.users))
		}
	}

	// GetOnlineDevice also resets the per-period IP bookkeeping the local device
	// limit counts against, so it runs every round whether or not the panel
	// takes the report. Skipping it let every IP a user ever used count towards
	// DeviceLimit until XrayR restarted.
	if !nodeDisabled && tag != "" {
		if onlineDevice, onlineErr := c.GetOnlineDevice(tag); onlineErr != nil {
			c.logger.Print(onlineErr)
		} else if c.capabilities.OnlineUserReport {
			if reporter, ok := c.apiClient.(api.OnlineUserReporter); ok {
				if onlineErr = reporter.ReportNodeOnlineUsers(onlineDevice); onlineErr != nil {
					c.logger.Print(onlineErr)
				} else if len(*onlineDevice) > 0 {
					c.logger.Printf("Reported %d online devices", len(*onlineDevice))
				}
			}
		}
	}

	if c.capabilities.IllegalReport {
		if reporter, ok := c.apiClient.(api.IllegalReporter); ok {
			if detectResult, detectErr := c.GetDetectResult(tag); detectErr != nil {
				c.logger.Print(detectErr)
			} else if len(*detectResult) > 0 {
				if detectErr = reporter.ReportIllegal(detectResult); detectErr != nil {
					c.logger.Print(detectErr)
				} else {
					c.logger.Printf("Report %d illegal behaviors", len(*detectResult))
				}
			}
		}
	}
	return nil
}

func (c *Controller) buildNodeTag(node *api.NodeInfo) string {
	return fmt.Sprintf("%s_%s_%d", node.NodeType, c.config.ListenIP, node.Port)
}

// certMonitor keeps the node certificate fresh and publishes its remaining
// validity. It is registered only for TLS nodes that manage their own certs.
func (c *Controller) certMonitor() error {
	node := c.currentNode()
	if node == nil || !node.EnableTLS || c.config.EnableREALITY || c.config.CertConfig == nil {
		return nil
	}
	if certFile := c.maintainCertificate(); certFile != "" {
		c.recordCertificateExpiry(node, certFile)
	}
	return nil
}

// maintainCertificate renews the node certificate when ACME is in use and
// returns the certificate file currently serving traffic. An empty return means
// no certificate is available yet.
func (c *Controller) maintainCertificate() string {
	certConfig := c.config.CertConfig
	switch certConfig.CertMode {
	case "dns", "http", "tls":
		lego, err := mylego.New(certConfig)
		if err != nil {
			c.logger.Print(err)
			return ""
		}
		// Xray-core supports the OcspStapling certification hot renew.
		certFile, _, _, err := lego.RenewCert()
		if err != nil {
			c.logger.Print(err)
			return ""
		}
		return certFile
	case "file":
		return certConfig.CertFile
	default:
		return ""
	}
}

// recordCertificateExpiry publishes how long the leaf certificate stays valid so
// an approaching expiry is visible before it starts breaking TLS handshakes.
func (c *Controller) recordCertificateExpiry(node *api.NodeInfo, certFile string) {
	days, err := certificateDaysRemaining(certFile)
	if err != nil {
		c.logger.WithError(err).Debug("Failed to read certificate expiry")
		return
	}
	observability.CertDaysRemaining.WithLabelValues(node.NodeType).Set(days)
}

const (
	// coreProbeTimeout bounds a single liveness dial.
	coreProbeTimeout = 3 * time.Second
	// coreProbeFailuresBeforeExit is how many consecutive failed probes are
	// needed before the node is judged dead. Three intervals is long enough that
	// a port move or a slow listener restart cannot produce a false positive.
	coreProbeFailuresBeforeExit = 3
)

// coreUnhealthy is invoked once the heartbeat has failed
// coreProbeFailuresBeforeExit times in a row. The default exits the process so
// the service manager restarts the node: xray-core offers no way to rebuild an
// instance in place, and every feature handle a controller holds (inbound,
// outbound, stats and policy managers) would be stale afterwards.
//
// It is a variable so tests can observe the decision without terminating.
var coreUnhealthy = func(nodeType, reason string) {
	log.WithFields(log.Fields{"node_type": nodeType, "reason": reason}).
		Error("Core is unresponsive; exiting so the service manager can restart the node")
	os.Exit(1)
}

// coreHeartbeatMonitor proves the core is still accepting connections.
//
// xray-core has no exported liveness flag, and a wedged core keeps the process
// alive while serving nothing, which neither the node nor the user monitor can
// see. Dialling the inbound listener is therefore the only end-to-end signal
// that the node is still doing its job.
func (c *Controller) coreHeartbeatMonitor() error {
	if c.config.DisableCoreHeartbeat {
		return nil
	}
	// Delay to start, so the first probe cannot race the initial listener setup.
	if time.Since(c.startAt) < c.pullPeriod {
		return nil
	}
	node := c.currentNode()
	if node == nil || node.Port == 0 {
		return nil
	}
	if err := c.probeInbound(node); err != nil {
		failures := c.coreFailures.Add(1)
		observability.CoreHealthy.WithLabelValues(node.NodeType).Set(0)
		observability.CoreProbeFailures.WithLabelValues(node.NodeType).Inc()
		c.logger.WithError(err).WithField("consecutive_failures", failures).Error("Core liveness probe failed")
		if failures >= coreProbeFailuresBeforeExit {
			coreUnhealthy(node.NodeType, err.Error())
		}
		return nil
	}
	c.coreFailures.Store(0)
	observability.CoreHealthy.WithLabelValues(node.NodeType).Set(1)
	return nil
}

// probeInbound dials the node's inbound listener. A completed TCP handshake is
// the signal; nothing is written, so the probe is never counted as user traffic.
func (c *Controller) probeInbound(node *api.NodeInfo) error {
	address := net.JoinHostPort(probeHost(c.config.ListenIP), strconv.Itoa(int(node.Port)))
	connection, err := net.DialTimeout("tcp", address, coreProbeTimeout)
	if err != nil {
		return err
	}
	// The handshake already proved liveness; a close failure adds nothing.
	_ = connection.Close()
	return nil
}

// probeHost turns a configured listen address into something dialable from the
// node itself.
func probeHost(listenIP string) string {
	switch listenIP {
	case "", "0.0.0.0":
		return "127.0.0.1"
	case "::", "[::]":
		return "::1"
	default:
		return listenIP
	}
}

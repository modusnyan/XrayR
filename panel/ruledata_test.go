package panel

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/xtls/xray-core/app/dispatcher"
	"github.com/xtls/xray-core/app/proxyman"
	"github.com/xtls/xray-core/common/serial"
	"github.com/xtls/xray-core/core"
	"github.com/xtls/xray-core/infra/conf"
)

// moduleRoot walks up from the working directory until it finds go.mod.
//
// `go test` runs the test binary with the package directory as the working
// directory, so a path a user typed relative to the repository root (./rules)
// would otherwise resolve to panel/rules. Resolving against the module root makes
// RULES_DAT_DIR behave the way the caller expects.
func moduleRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}
	for {
		if _, statErr := os.Stat(filepath.Join(dir, "go.mod")); statErr == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("could not find go.mod above %s", dir)
		}
		dir = parent
	}
}

// TestRuleDataLoads is the end-to-end check for the shipped routing template.
//
// geoip.dat / geosite.dat are not stored in the repository: they are fetched on
// every install by release/download-rules-dat.sh. This test builds the exact
// release/config/route.json against a directory holding those files and starts a
// real xray-core instance, so a template that references a rule set the downloaded
// data does not provide fails here instead of on a user's node.
//
// It is skipped unless RULES_DAT_DIR points at a directory containing geoip.dat and
// geosite.dat, so `go test ./...` stays offline-friendly. The path may be absolute
// or relative to the repository root:
//
//	bash release/download-rules-dat.sh ./rules
//	RULES_DAT_DIR=./rules go test ./panel/
func TestRuleDataLoads(t *testing.T) {
	assetDir := os.Getenv("RULES_DAT_DIR")
	if assetDir == "" {
		t.Skip("RULES_DAT_DIR is not set; run release/download-rules-dat.sh first")
	}
	root := moduleRoot(t)
	if !filepath.IsAbs(assetDir) {
		assetDir = filepath.Join(root, assetDir)
	}
	assetDir = filepath.Clean(assetDir)

	for _, name := range []string{"geoip.dat", "geosite.dat"} {
		if _, statErr := os.Stat(filepath.Join(assetDir, name)); statErr != nil {
			t.Fatalf("RULES_DAT_DIR %s does not contain %s: %v", assetDir, name, statErr)
		}
	}
	t.Setenv("XRAY_LOCATION_ASSET", assetDir)

	raw, err := os.ReadFile(filepath.Join(root, "release", "config", "route.json"))
	if err != nil {
		t.Fatalf("read route.json: %v", err)
	}
	routerConf := &conf.RouterConfig{}
	if err := json.Unmarshal(raw, routerConf); err != nil {
		t.Fatalf("unmarshal route.json: %v", err)
	}
	routeConfig, err := routerConf.Build()
	if err != nil {
		t.Fatalf("build route config against %s: %v", assetDir, err)
	}

	cfg := &core.Config{
		App: []*serial.TypedMessage{
			serial.ToTypedMessage(&dispatcher.Config{}),
			serial.ToTypedMessage(&proxyman.InboundConfig{}),
			serial.ToTypedMessage(&proxyman.OutboundConfig{}),
			serial.ToTypedMessage(routeConfig),
		},
	}
	instance, err := core.New(cfg)
	if err != nil {
		t.Fatalf("create core instance against %s: %v", assetDir, err)
	}
	if err := instance.Start(); err != nil {
		t.Fatalf("start core instance against %s: %v", assetDir, err)
	}
	if err := instance.Close(); err != nil {
		t.Fatalf("close core instance: %v", err)
	}
}

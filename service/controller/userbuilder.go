package controller

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/sagernet/sing-shadowsocks/shadowaead_2022"
	C "github.com/sagernet/sing/common"
	"github.com/xtls/xray-core/common/errors"
	"github.com/xtls/xray-core/common/protocol"
	"github.com/xtls/xray-core/common/serial"
	"github.com/xtls/xray-core/infra/conf"
	"github.com/xtls/xray-core/proxy/shadowsocks"
	"github.com/xtls/xray-core/proxy/shadowsocks_2022"
	"github.com/xtls/xray-core/proxy/trojan"
	"github.com/xtls/xray-core/proxy/vless"

	"github.com/XrayR-project/XrayR/api"
)

func (c *Controller) buildVmessUser(userInfo *[]api.UserInfo, tag string) (users []*protocol.User) {
	users = make([]*protocol.User, len(*userInfo))
	for i, user := range *userInfo {
		vmessAccount := &conf.VMessAccount{
			ID:       user.UUID,
			Security: "auto",
		}
		users[i] = &protocol.User{
			Level:   0,
			Email:   buildUserTag(tag, &user), // Email: InboundTag|email|uid
			Account: serial.ToTypedMessage(vmessAccount.Build()),
		}
	}
	return users
}

func (c *Controller) buildVlessUser(userInfo *[]api.UserInfo, tag string, flow string) (users []*protocol.User) {
	users = make([]*protocol.User, len(*userInfo))
	for i, user := range *userInfo {
		vlessAccount := &vless.Account{
			Id:   user.UUID,
			Flow: flow,
		}
		users[i] = &protocol.User{
			Level:   0,
			Email:   buildUserTag(tag, &user),
			Account: serial.ToTypedMessage(vlessAccount),
		}
	}
	return users
}

func (c *Controller) buildTrojanUser(userInfo *[]api.UserInfo, tag string) (users []*protocol.User) {
	users = make([]*protocol.User, len(*userInfo))
	for i, user := range *userInfo {
		trojanAccount := &trojan.Account{
			Password: user.UUID,
		}
		users[i] = &protocol.User{
			Level:   0,
			Email:   buildUserTag(tag, &user),
			Account: serial.ToTypedMessage(trojanAccount),
		}
	}
	return users
}

func (c *Controller) buildSSUser(userInfo *[]api.UserInfo, method string, tag string) (users []*protocol.User) {
	users = make([]*protocol.User, len(*userInfo))

	for i, user := range *userInfo {
		// shadowsocks2022 Key = "openssl rand -base64 32" and multi users needn't cipher method
		if C.Contains(shadowaead_2022.List, strings.ToLower(method)) {
			e := buildUserTag(tag, &user)
			userKey, err := c.checkShadowsocksPassword(user.Passwd, method)
			if err != nil {
				errors.LogError(context.Background(), "[UID: %d] %s", user.UID, err)
				continue
			}
			users[i] = &protocol.User{
				Level: 0,
				Email: e,
				Account: serial.ToTypedMessage(&shadowsocks_2022.Account{
					Key: userKey,
				}),
			}
		} else {
			users[i] = &protocol.User{
				Level: 0,
				Email: buildUserTag(tag, &user),
				Account: serial.ToTypedMessage(&shadowsocks.Account{
					Password:   user.Passwd,
					CipherType: cipherFromString(method),
				}),
			}
		}
	}
	return users
}

func cipherFromString(c string) shadowsocks.CipherType {
	switch strings.ToLower(c) {
	case "aes-128-gcm", "aead_aes_128_gcm":
		return shadowsocks.CipherType_AES_128_GCM
	case "aes-256-gcm", "aead_aes_256_gcm":
		return shadowsocks.CipherType_AES_256_GCM
	case "chacha20-poly1305", "aead_chacha20_poly1305", "chacha20-ietf-poly1305":
		return shadowsocks.CipherType_CHACHA20_POLY1305
	case "xchacha20-poly1305", "aead_xchacha20_poly1305":
		return shadowsocks.CipherType_XCHACHA20_POLY1305
	case "none", "plain":
		return shadowsocks.CipherType_NONE
	default:
		return shadowsocks.CipherType_UNKNOWN
	}
}

// buildUserTag builds the per-user email tag used by xray-core stats and limiters.
// It is a free function so callers pass the tag explicitly instead of reading
// shared Controller state.
func buildUserTag(tag string, user *api.UserInfo) string {
	return fmt.Sprintf("%s|%s|%d", tag, user.Email, user.UID)
}

func (c *Controller) checkShadowsocksPassword(password string, method string) (string, error) {
	if c.capabilities.Shadowsocks2022KeyDerivation {
		var userKey string
		if len(password) < 16 {
			return "", newError("shadowsocks2022 key's length must be greater than 16").AtWarning()
		}
		if method == "2022-blake3-aes-128-gcm" {
			userKey = password[:16]
		} else {
			if len(password) < 32 {
				return "", newError("shadowsocks2022 key's length must be greater than 32").AtWarning()
			}
			userKey = password[:32]
		}
		return base64.StdEncoding.EncodeToString([]byte(userKey)), nil
	} else {
		return password, nil
	}
}

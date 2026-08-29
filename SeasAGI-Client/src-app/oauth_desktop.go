package main

import (
	"context"
	"fmt"
	"html"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/SeasAGI/SeasAGI-Client/internal/config"
	"github.com/SeasAGI/SeasAGI-Client/internal/oauth"
)

const oauthCallbackAddr = "127.0.0.1:43819"

type pendingOAuthFlow struct {
	ProviderName string
	State        string
	PKCE         *oauth.PKCEFlow
	Config       *oauth.OAuthConfig
	Status       string
	Error        string
	StartedAt    time.Time
	Server       *http.Server
	Listener     net.Listener
}

func (a *App) restoreOAuthProviderConfigs() {
	for _, saved := range a.configSvc.ListOAuthProviderConfigs() {
		providerInfo, ok := oauth.GetOAuthProvider(saved.ProviderName)
		if !ok {
			continue
		}
		a.oauthRefresh.RegisterProvider(saved.ProviderName, &oauth.OAuthConfig{
			ClientID:     saved.ClientID,
			ClientSecret: saved.ClientSecret,
			AuthURL:      providerInfo.AuthURL,
			TokenURL:     providerInfo.TokenURL,
			Scopes:       providerInfo.Scopes,
		})
	}
}

func (a *App) resolveOAuthProviderConfig(providerName, clientID, clientSecret, redirectURI string) (*oauth.OAuthConfig, error) {
	providerInfo, ok := oauth.GetOAuthProvider(providerName)
	if !ok {
		return nil, fmt.Errorf("unknown OAuth provider: %s", providerName)
	}
	if strings.TrimSpace(clientID) == "" {
		if saved, exists := a.configSvc.GetOAuthProviderConfig(providerName); exists {
			clientID = saved.ClientID
			if strings.TrimSpace(clientSecret) == "" {
				clientSecret = saved.ClientSecret
			}
		}
	}
	clientID = strings.TrimSpace(clientID)
	clientSecret = strings.TrimSpace(clientSecret)
	if clientID == "" {
		return nil, fmt.Errorf("%s client id is required", providerInfo.DisplayName)
	}
	cfg := &oauth.OAuthConfig{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		AuthURL:      providerInfo.AuthURL,
		TokenURL:     providerInfo.TokenURL,
		RedirectURI:  redirectURI,
		Scopes:       providerInfo.Scopes,
	}
	if err := a.configSvc.SaveOAuthProviderConfig(config.OAuthProviderConfig{
		ProviderName: providerName,
		ClientID:     clientID,
		ClientSecret: clientSecret,
	}); err != nil {
		return nil, err
	}
	a.oauthRefresh.RegisterProvider(providerName, cfg)
	return cfg, nil
}

func (a *App) createOAuthCallbackServer(flow *pendingOAuthFlow) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/oauth/callback", func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		if query.Get("state") != flow.State {
			flow.Status = "error"
			flow.Error = "oauth state mismatch"
			writeOAuthCallbackPage(w, false, flow.Error)
			return
		}
		if authErr := query.Get("error"); authErr != "" {
			flow.Status = "error"
			flow.Error = authErr
			writeOAuthCallbackPage(w, false, flow.Error)
			return
		}
		code := strings.TrimSpace(query.Get("code"))
		if code == "" {
			flow.Status = "error"
			flow.Error = "oauth code missing"
			writeOAuthCallbackPage(w, false, flow.Error)
			return
		}

		resp, err := oauth.ExchangeCode(flow.Config, code, flow.PKCE)
		if err != nil {
			flow.Status = "error"
			flow.Error = err.Error()
			writeOAuthCallbackPage(w, false, flow.Error)
			return
		}
		if err := a.oauthStore.Save(flow.ProviderName, oauth.TokenResponseToInfo(resp)); err != nil {
			flow.Status = "error"
			flow.Error = err.Error()
			writeOAuthCallbackPage(w, false, flow.Error)
			return
		}

		flow.Status = "connected"
		flow.Error = ""
		writeOAuthCallbackPage(w, true, flow.ProviderName)
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			_ = flow.Server.Shutdown(ctx)
		}()
	})

	listener, err := net.Listen("tcp", oauthCallbackAddr)
	if err != nil {
		return err
	}
	flow.Listener = listener
	flow.Config.RedirectURI = fmt.Sprintf("http://%s/oauth/callback", listener.Addr().String())
	flow.Server = &http.Server{Handler: mux}
	go func() {
		if err := flow.Server.Serve(listener); err != nil && err != http.ErrServerClosed {
			flow.Status = "error"
			flow.Error = err.Error()
		}
	}()
	return nil
}

func writeOAuthCallbackPage(w http.ResponseWriter, success bool, message string) {
	status := "授权成功"
	detail := "你可以关闭这个页面并返回 SeasAGI。"
	if !success {
		status = "授权失败"
		detail = message
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = fmt.Fprintf(w, `<!doctype html>
<html><head><meta charset="utf-8"><title>SeasAGI OAuth</title></head>
<body style="font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',sans-serif;background:#0f172a;color:#f1f5f9;padding:32px;">
<div style="max-width:540px;margin:0 auto;background:#1e293b;border:1px solid #334155;border-radius:12px;padding:24px;">
<h2 style="margin:0 0 12px;">%s</h2>
<p style="margin:0;color:#94a3b8;line-height:1.7;">%s</p>
</div></body></html>`, html.EscapeString(status), html.EscapeString(detail))
}

func maskClientID(value string) string {
	if len(value) <= 8 {
		return value
	}
	return value[:4] + "..." + value[len(value)-4:]
}

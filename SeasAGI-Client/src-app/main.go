package main

import (
	"context"
	"embed"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	gsync "sync"
	"sync/atomic"
	"syscall"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/menu"
	"github.com/wailsapp/wails/v2/pkg/menu/keys"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/mac"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
	"github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/SeasAGI/SeasAGI-Client/internal/auth"
	"github.com/SeasAGI/SeasAGI-Client/internal/config"
	"github.com/SeasAGI/SeasAGI-Client/internal/configio"
	"github.com/SeasAGI/SeasAGI-Client/internal/deeplink"
	"github.com/SeasAGI/SeasAGI-Client/internal/discovery"
	"github.com/SeasAGI/SeasAGI-Client/internal/gateway"
	"github.com/SeasAGI/SeasAGI-Client/internal/localtoken"
	"github.com/SeasAGI/SeasAGI-Client/internal/logging"
	"github.com/SeasAGI/SeasAGI-Client/internal/logs"
	"github.com/SeasAGI/SeasAGI-Client/internal/mcp"
	"github.com/SeasAGI/SeasAGI-Client/internal/mitm"
	"github.com/SeasAGI/SeasAGI-Client/internal/network"
	"github.com/SeasAGI/SeasAGI-Client/internal/optimizer"
	"github.com/SeasAGI/SeasAGI-Client/internal/presets"
	"github.com/SeasAGI/SeasAGI-Client/internal/prompts"
	"github.com/SeasAGI/SeasAGI-Client/internal/sessions"
	"github.com/SeasAGI/SeasAGI-Client/internal/skills"
	"github.com/SeasAGI/SeasAGI-Client/internal/sync"
	"github.com/SeasAGI/SeasAGI-Client/internal/tray"
	"github.com/SeasAGI/SeasAGI-Client/internal/usage"
	"github.com/SeasAGI/SeasAGI-Client/internal/webui"
)

//go:embed all:frontend/dist
var assets embed.FS

//go:embed assets/tray_icon.png
var trayIcon []byte

var quitting int32

func main() {
	if os.Getenv("SEASAGI_WEB") == "1" || len(os.Args) > 1 && os.Args[1] == "--web" {
		runWebUI()
		return
	}
	runDesktop()
}

func runWebUI() {
	cfg, _ := config.LoadOrDefault()
	port := cfg.ListenPort + 1000
	if v := os.Getenv("SEASAGI_WEB_PORT"); v != "" {
		fmt.Sscanf(v, "%d", &port)
	}

	srv := webui.NewServer(assets, port)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := srv.Start(ctx); err != nil {
		logging.Errorf("Failed to start Web UI: %v", err)
		os.Exit(1)
	}

	logging.Infof("SeasAGI Web UI: %s", srv.URL())
	logging.Info("Press Ctrl+C to stop")

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	srv.Stop()
}

func runDesktop() {
	cfg, _ := config.LoadOrDefault()
	localTokenStore, err := localtoken.NewStore()
	if err != nil {
		logging.Warningf("local token store init failed: %v — continuing without persistence", err)
	} else {
		defer localTokenStore.Close()
	}

	var accessToken string
	if localTokenStore != nil {
		accessToken, _ = localTokenStore.GetOrCreate()
	}
	lifecycleCtx, lifecycleCancel := context.WithCancel(context.Background())
	var shutdownOnce gsync.Once

	authSvc := auth.NewService()
	configSvc := config.NewService(cfg)
	logSvc := logs.NewService()
	discoverySvc := discovery.NewService(configSvc)
	gatewaySvc := gateway.NewService(cfg.ListenPort, accessToken, configSvc, authSvc, logSvc)
	mitmCADir := filepath.Join(mustHomeDir(), ".seasagi", "mitm")
	mitmCA, mitmCAErr := mitm.NewCA(mitmCADir)
	if mitmCAErr != nil {
		logging.Warningf("MITM CA init failed: %v", mitmCAErr)
	}
	mitmRules := mitm.NewDefaultRules()
	mitmGatewayURL := fmt.Sprintf("http://127.0.0.1:%d", cfg.ListenPort)
	mitmMgr := mitm.NewManager(mitmCA, mitmRules, mitmGatewayURL)
	if mitmCA != nil {
		mitmMgr.SetTrustInstaller(mitm.NewTrustInstaller())
		mitmMgr.SetSystemProxySetter(network.NewSystemProxySetter())
	}
	mcpSvc := mcp.NewService()
	promptsSvc := prompts.NewService()
	skillsSvc := skills.NewService()
	usageSvc := usage.NewService()
	deeplinkMgr := deeplink.NewManager()
	presetsSvc := presets.NewService()
	syncMgr := sync.NewManager(sync.SyncConfig{})
	sessionsSvc := sessions.NewService()
	configioSvc := configio.NewService(configSvc)
	optimizerSvc := optimizer.NewService(configSvc, usageSvc)
	app := NewApp(authSvc, configSvc, gatewaySvc, logSvc, discoverySvc, mcpSvc, promptsSvc, skillsSvc, usageSvc, optimizerSvc, deeplinkMgr, presetsSvc, syncMgr, sessionsSvc, configioSvc, localTokenStore)
	app.SetMITMManager(mitmMgr)

	trayMgr := tray.NewManager(
		func(channelID string) {
			configSvc.SetDefaultModel("", channelID)
		},
		func() {
			runtime.WindowShow(app.ctx)
		},
		func() {
			atomic.StoreInt32(&quitting, 1)
			runtime.Quit(app.ctx)
		},
	)

	shutdownDeps := func() {
		shutdownOnce.Do(func() {
			lifecycleCancel()
			trayMgr.Stop()
			_ = app.StopTunnel()
			_ = app.StopMITM()
			app.oauthRefresh.Stop()
			gatewaySvc.Stop()
		})
	}
	defer shutdownDeps()

	// 兜底处理桌面进程退出信号，确保 MITM 接管在进程结束时回滚为未接管状态。
	desktopSigCh := make(chan os.Signal, 1)
	signal.Notify(desktopSigCh, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP, syscall.SIGQUIT)
	defer signal.Stop(desktopSigCh)
	go func() {
		<-desktopSigCh
		atomic.StoreInt32(&quitting, 1)
		shutdownDeps()
		os.Exit(0)
	}()

	appMenu := buildAppMenu(app, gatewaySvc)

	go func() {
		chs, _ := configSvc.ListChannels()
		infos := make([]tray.ChannelInfo, 0, len(chs))
		for _, ch := range chs {
			infos = append(infos, tray.ChannelInfo{
				ID:          ch.ChannelID,
				DisplayName: ch.DisplayName,
				Enabled:     ch.Enabled,
			})
		}
		trayMgr.UpdateChannels(infos)
	}()

	err = wails.Run(&options.App{
		Title:  "SeasAGI",
		Width:  1230,
		Height: 720,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 26, G: 26, B: 46, A: 1},
		OnStartup: func(ctx context.Context) {
			app.startup(ctx)
			go trayMgr.Run(trayIcon)
			network.OnWake(func() {
				if !gatewaySvc.IsRunning() {
					go gatewaySvc.Start(lifecycleCtx)
				}
			})
			network.OnNetworkChange(func() {
				if gatewaySvc.IsRunning() {
					gatewaySvc.Stop()
					go gatewaySvc.Start(lifecycleCtx)
				}
			})
			if authSvc.IsLoggedIn() {
				go gatewaySvc.Start(lifecycleCtx)
			}
			go network.WatchSystemEvents(lifecycleCtx)
		},
		OnShutdown: func(_ context.Context) {
			shutdownDeps()
		},
		OnBeforeClose: func(ctx context.Context) bool {
			if atomic.LoadInt32(&quitting) == 1 {
				return false
			}
			atomic.StoreInt32(&quitting, 1)
			go runtime.Quit(ctx)
			return true
		},
		Bind: []interface{}{
			app,
			authSvc,
			configSvc,
			gatewaySvc,
			logSvc,
			discoverySvc,
			mcpSvc,
			promptsSvc,
			skillsSvc,
			usageSvc,
			presetsSvc,
			sessionsSvc,
		},
		Menu: appMenu,
		Mac: &mac.Options{
			TitleBar: &mac.TitleBar{
				HideTitleBar: false,
			},
			About: &mac.AboutInfo{
				Title:   "SeasAGI",
				Message: "本地大模型通道切换客户端\nVersion 0.1.0",
			},
			WebviewIsTransparent: false,
		},
		Windows: &windows.Options{
			WebviewIsTransparent: false,
		},
		SingleInstanceLock: &options.SingleInstanceLock{
			UniqueId: "com.seasagi.desktop",
		},
	})

	if err != nil {
		println("Error:", err.Error())
	}
}

func buildAppMenu(app *App, gw *gateway.Service) *menu.Menu {
	appMenu := menu.NewMenu()

	fileMenu := appMenu.AddSubmenu("SeasAGI")
	fileMenu.AddText("关于 SeasAGI", keys.CmdOrCtrl(""), func(_ *menu.CallbackData) {
		runtime.MessageDialog(app.ctx, runtime.MessageDialogOptions{
			Type:    runtime.InfoDialog,
			Title:   "关于 SeasAGI",
			Message: "SeasAGI v0.1.0\n本地大模型通道切换客户端\n\n本地网关: http://127.0.0.1:4318/v1",
		})
	})
	fileMenu.AddSeparator()
	fileMenu.AddText("复制网关地址", keys.CmdOrCtrl("c"), func(_ *menu.CallbackData) {
		token, _ := app.GetLocalAccessToken()
		text := fmt.Sprintf("Base URL: http://127.0.0.1:%d/v1\nAPI Key: %s", gw.GetListenPort(), token)
		runtime.ClipboardSetText(app.ctx, text)
	})
	fileMenu.AddSeparator()
	fileMenu.AddText("隐藏窗口", keys.CmdOrCtrl("h"), func(_ *menu.CallbackData) {
		runtime.WindowHide(app.ctx)
	})
	fileMenu.AddText("显示窗口", keys.CmdOrCtrl("j"), func(_ *menu.CallbackData) {
		runtime.WindowShow(app.ctx)
	})
	fileMenu.AddSeparator()
	fileMenu.AddText("退出", keys.CmdOrCtrl("q"), func(_ *menu.CallbackData) {
		atomic.StoreInt32(&quitting, 1)
		runtime.Quit(app.ctx)
	})

	editMenu := appMenu.AddSubmenu("编辑")
	editMenu.AddText("剪切", keys.CmdOrCtrl("x"), func(_ *menu.CallbackData) {})
	editMenu.AddText("复制", keys.CmdOrCtrl("c"), func(_ *menu.CallbackData) {})
	editMenu.AddText("粘贴", keys.CmdOrCtrl("v"), func(_ *menu.CallbackData) {})
	editMenu.AddText("全选", keys.CmdOrCtrl("a"), func(_ *menu.CallbackData) {})

	viewMenu := appMenu.AddSubmenu("视图")
	viewMenu.AddText("重新加载", keys.CmdOrCtrl("r"), func(_ *menu.CallbackData) {
		runtime.WindowReload(app.ctx)
	})

	return appMenu
}

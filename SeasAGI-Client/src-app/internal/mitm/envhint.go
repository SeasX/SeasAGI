package mitm

import (
	"os"
	"runtime"
	"strings"
)

// EnvHint 返回当前 Shell 环境下的代理环境变量设置/取消命令。
type EnvHint struct {
	Shell      string `json:"shell"`
	ExportCmds string `json:"export_cmds"`
	UnsetCmds  string `json:"unset_cmds"`
}

// DetectShellEnv 根据 SHELL 环境变量检测当前 shell 类型，返回对应的 export/unset 命令。
func DetectShellEnv(proxyAddr string) EnvHint {
	shell := detectShellType()

	switch shell {
	case "fish":
		return EnvHint{
			Shell:      "fish",
			ExportCmds: "set -x HTTP_PROXY " + proxyAddr + "; set -x HTTPS_PROXY " + proxyAddr,
			UnsetCmds:  "set -e HTTP_PROXY; set -e HTTPS_PROXY",
		}
	case "powershell":
		return EnvHint{
			Shell:      "powershell",
			ExportCmds: "$env:HTTP_PROXY='" + proxyAddr + "'; $env:HTTPS_PROXY='" + proxyAddr + "'",
			UnsetCmds:  "Remove-Item Env:HTTP_PROXY; Remove-Item Env:HTTPS_PROXY",
		}
	case "cmd":
		return EnvHint{
			Shell:      "cmd",
			ExportCmds: "set HTTP_PROXY=" + proxyAddr + " && set HTTPS_PROXY=" + proxyAddr,
			UnsetCmds:  "set HTTP_PROXY= && set HTTPS_PROXY=",
		}
	default: // bash, zsh, sh 等 POSIX shell
		return EnvHint{
			Shell:      shell,
			ExportCmds: "export HTTP_PROXY=" + proxyAddr + " HTTPS_PROXY=" + proxyAddr,
			UnsetCmds:  "unset HTTP_PROXY HTTPS_PROXY",
		}
	}
}

// detectShellType 返回 shell 类型字符串。
func detectShellType() string {
	if runtime.GOOS == "windows" {
		// Windows 下检查是否是 PowerShell 或 cmd
		if _, ok := os.LookupEnv("PSModulePath"); ok {
			return "powershell"
		}
		return "cmd"
	}

	shellPath := os.Getenv("SHELL")
	if shellPath == "" {
		return "bash" // 回退到 bash 语法
	}

	// 取 shell 基名
	parts := strings.Split(shellPath, "/")
	shellName := parts[len(parts)-1]

	switch shellName {
	case "fish":
		return "fish"
	case "zsh":
		return "zsh"
	case "bash":
		return "bash"
	case "sh":
		return "sh"
	default:
		return "bash" // 未知 shell 回退到 bash 语法
	}
}

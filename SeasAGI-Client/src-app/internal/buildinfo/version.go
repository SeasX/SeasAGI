package buildinfo

// Version 是客户端版本号，默认值与 scripts/version.txt 保持一致。
// 构建脚本通过 -ldflags "-X github.com/SeasAGI/SeasAGI-Client/internal/buildinfo.Version=$VERSION"
// 在编译期覆盖，运行时无需读取文件。
var Version = "0.2.5"

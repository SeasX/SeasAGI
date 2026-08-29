cask "seasagi" do
  version "0.1.0"
  sha256 "TBD"

  url "https://github.com/SeasX/SeasAGI/releases/download/v#{version}/SeasAGI.dmg"
  name "SeasAGI"
  desc "Local LLM API switching client for macOS"
  homepage "https://github.com/SeasX/SeasAGI"

  livecheck do
    url :url
    strategy :github_latest
  end

  depends_on macos: ">= :ventura"

  app "SeasAGI.app"

  zap trash: [
    "~/Library/Application Support/SeasAGI",
    "~/Library/Caches/SeasAGI",
    "~/Library/Preferences/com.seasagi.desktop.plist",
    "~/Library/Logs/SeasAGI",
  ]

  caveats do
    <<~EOS
      SeasAGI requires macOS 13 (Ventura) or later.

      After installation, SeasAGI will be available in your Applications folder
      and can be launched from Spotlight or the command line:

        open -a SeasAGI

      The local gateway will be available at http://127.0.0.1:4318/v1
    EOS
  end
end

# Homebrew formula for katazuke
# TODO: Move this to agrahamlincoln/homebrew-katazuke repository
# This file will be removed from the main repo once the tap repo is set up

class Katazuke < Formula
  desc "Developer workspace maintenance tool for tidying up git repositories"
  homepage "https://github.com/agrahamlincoln/katazuke"
  version "0.1.0"

  # These URLs and SHAs will be updated by the release process
  if OS.mac?
    url "https://github.com/agrahamlincoln/katazuke/releases/download/v0.1.0/katazuke-0.1.0-darwin-arm64.tar.gz"
    sha256 "TODO: Update with actual SHA256"
  elsif OS.linux?
    url "https://github.com/agrahamlincoln/katazuke/releases/download/v0.1.0/katazuke-0.1.0-linux-amd64.tar.gz"
    sha256 "TODO: Update with actual SHA256"
  end

  def install
    bin.install "katazuke"
  end

  test do
    system "#{bin}/katazuke", "--version"
  end
end

# Installation

## macOS with [Homebrew](https://brew.sh)
```
brew install --cask mrclmr/tap/w2a
```

## Windows with [Scoop](https://scoop.sh)
```
scoop bucket add mrclmr-bucket https://github.com/mrclmr/scoop-bucket.git
scoop install w2a
```

## Manual

`w2a` needs these programs:
* [`sox`](https://sourceforge.net/projects/sox/)
* [`espeak-ng`](https://github.com/espeak-ng/espeak-ng) (or on macOS pre-installed `say`)
* [`ffmpeg`](https://ffmpeg.org) (or on macOS pre-installed `afconvert`)

### Go
```
go install github.com/mrclmr/w2a@latest
```

### Download binary

See binaries in the [Releases](https://github.com/mrclmr/w2a/releases) section.

### Completions and man pages
```
w2a completion -h
```
```
w2a man -h
```
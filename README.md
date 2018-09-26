# slackv

Slack viewer

Available to use for Linux, macOS and Windows.

# How to build

```
$ go get github.com/BurntSushi/toml github.com/fsnotify/fsnotify golang.org/x/net/websocket
$ git clone https://github.com/yoffy/slackv.git
$ cd slackv
$ go build slackv
```

# Write config

```
$ cp config.toml.sample config.toml
$ vim config.toml
```

# Run

```
$ ./slackv
```

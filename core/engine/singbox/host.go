package singbox

import "errors"

var errTunHostUnsupported = errors.New("elevated TUN host is only supported on Windows")

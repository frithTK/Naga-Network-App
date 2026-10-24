import Foundation
#if os(macOS)
import Darwin
#else
import Glibc
#endif

// One newline-terminated frame; stdin stays open as the session lifetime pipe.
// FileHandle.read(upToCount:) can wait to fill its buffer on a pipe, even after
// the newline arrived. A single POSIX read returns the bytes currently available.
func readVPNConfig(from input: FileHandle) throws -> Data {
  var data = Data()
  var buffer = [UInt8](repeating: 0, count: 65536)
  while data.count <= vpnConfigLimit {
    let count = buffer.withUnsafeMutableBytes { bytes in
      read(input.fileDescriptor, bytes.baseAddress!, min(bytes.count, vpnConfigLimit + 1 - data.count))
    }
    if count < 0 {
      if errno == EINTR { continue }
      throw VPNFailure("Не удалось прочитать конфигурацию VPN")
    }
    guard count > 0 else { throw VPNFailure("Не получена конфигурация VPN") }
    let chunk = buffer.prefix(count)
    if let newline = chunk.firstIndex(of: 10) {
      data.append(contentsOf: chunk.prefix(upTo: newline))
      guard !data.isEmpty else { throw VPNFailure("Не получена конфигурация VPN") }
      return data
    }
    data.append(contentsOf: chunk)
  }
  throw VPNFailure("Конфигурация VPN превышает 8 МБ")
}

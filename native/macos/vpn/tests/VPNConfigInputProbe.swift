import Foundation

// Exercise the production frame reader in a real child with its stdin open.
@main enum VPNConfigInputProbe {
  static func main() {
    do {
      let data = try readVPNConfig(from: .standardInput)
      // Echo bytes for exact content comparison, without logging real profiles.
      print(data.base64EncodedString())
      fflush(stdout)
      // Readiness must leave stdin open for the subsequent EOF/stop signal.
      while let chunk = try FileHandle.standardInput.read(upToCount: 1), !chunk.isEmpty {}
      print("stopped")
    } catch {
      print("error")
    }
  }
}

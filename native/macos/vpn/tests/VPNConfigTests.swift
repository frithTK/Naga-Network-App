import Foundation

@main enum VPNConfigTests {
  static func main() throws {
    let base: [String: Any] = [
      "inbounds": [["type": "tun", "tag": "tun-in", "interface_name": "untrusted",
        "auto_route": true, "include_uid": [0], "mtu": 9000]],
      "outbounds": [["type": "vless", "tag": "proxy", "server": "example.invalid", "server_port": 443,
        "uuid": "00000000-0000-4000-8000-000000000001", "tls": ["enabled": true,
          "server_name": "example.invalid", "reality": ["enabled": true,
            "public_key": "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA", "short_id": "0123456789abcdef"]]]],
      "route": ["final": "proxy", "rules": [["port": 53, "action": "hijack-dns"]]],
      "dns": ["servers": [["type": "udp", "tag": "bootstrap", "server": "1.1.1.1"]], "final": "bootstrap"],
      "experimental": ["clash_api": ["external_controller": "127.0.0.1:49167", "secret": String(repeating: "a", count: 32)]]
    ]
    func prepare(_ value: [String: Any], interface: String = "utun124") throws -> Data {
      try VPNConfig.prepare(JSONSerialization.data(withJSONObject: value), interface: interface)
    }
    let prepared = try prepare(base)
    let output = try JSONSerialization.jsonObject(with: prepared) as! [String: Any]
    let tun = (output["inbounds"] as! [[String: Any]])[0]
    precondition(tun["interface_name"] as? String == "utun124")
    precondition(tun["auto_route"] as? Bool == false && tun["mtu"] as? Int == 1500)
    precondition(tun["include_uid"] == nil && tun["stack"] as? String == "gvisor")
    let route = output["route"] as! [String: Any]
    precondition(route["auto_detect_interface"] as? Bool == true)
    precondition((route["rules"] as! [[String: Any]])[0]["action"] as? String == "hijack-dns")
    var count = 1
    func reject(_ name: String, _ mutate: (inout [String: Any]) -> Void) throws {
      var value = base
      mutate(&value)
      do { _ = try prepare(value) } catch { count += 1; return }
      throw VPNFailure("accepted forbidden config: \(name)")
    }
    try reject("arbitrary service") { $0["services"] = [["type": "ssm-api", "listen": "0.0.0.0"]] }
    try reject("endpoint") { $0["endpoints"] = [["type": "wireguard"]] }
    try reject("certificate file") {
      $0["outbounds"] = [["type": "trojan", "tls": ["certificate_path": "/etc/secret"]]]
    }
    try reject("root cache") { $0["experimental"] = ["cache_file": ["enabled": true, "path": "/etc/secret"]] }
    try reject("external UI") {
      $0["experimental"] = ["clash_api": ["external_ui": "/etc", "external_controller": "127.0.0.1:49167"]]
    }
    try reject("public API") {
      $0["experimental"] = ["clash_api": ["external_controller": "0.0.0.0:49167", "secret": String(repeating: "a", count: 32)]]
    }
    try reject("reserved API port") {
      $0["experimental"] = ["clash_api": ["external_controller": "127.0.0.1:0053", "secret": String(repeating: "a", count: 32)]]
    }
    try reject("weak API secret") {
      $0["experimental"] = ["clash_api": ["external_controller": "127.0.0.1:49167", "secret": "short"]]
    }
    try reject("public inbound") { $0["inbounds"] = [["type": "tun", "tag": "tun"], ["type": "mixed", "listen": "0.0.0.0", "listen_port": 8080]] }
    try reject("privileged inbound port") { $0["inbounds"] = [["type": "tun", "tag": "tun"], ["type": "mixed", "listen": "127.0.0.1", "listen_port": 53]] }
    try reject("two tunnels") { $0["inbounds"] = [["type": "tun", "tag": "a"], ["type": "tun", "tag": "b"]] }
    try reject("no tunnel") { $0["inbounds"] = [] }
    try reject("root file rule-set") { $0["route"] = ["rule_set": [["type": "local", "tag": "r", "path": "/etc/secret"]]] }
    try reject("remote rule-set") { $0["route"] = ["rule_set": [["type": "remote", "tag": "r", "url": "https://example.invalid"]]] }
    try reject("root DNS hosts file") { $0["dns"] = ["servers": [["type": "hosts", "path": ["/etc/secret"]]]] }
    try reject("unsupported outbound") { $0["outbounds"] = [["type": "ssh", "private_key_path": "/etc/secret"]] }
    try reject("nested outbound field") { $0["outbounds"] = [["type": "direct", "server": ["file": "/etc/secret"]]] }
    var inline = base
    inline["route"] = ["rule_set": [["type": "inline", "tag": "r", "rules": [["domain_suffix": ["example.invalid"]]]]]]
    _ = try prepare(inline); count += 1
    var mixed = base
    mixed["inbounds"] = [["type": "tun", "tag": "tun"], ["type": "mixed", "listen": "127.0.0.1", "listen_port": 2080]]
    _ = try prepare(mixed); count += 1
    for name in ["en0", "utun1;id", "utun12345"] {
      do { _ = try prepare(base, interface: name) } catch { count += 1; continue }
      throw VPNFailure("accepted invalid interface")
    }
    var oversizedRejected = false
    do { _ = try VPNConfig.prepare(Data(repeating: 32, count: vpnConfigLimit + 1), interface: "utun124") }
    catch { oversizedRejected = true; count += 1 }
    precondition(oversizedRejected)
    if CommandLine.arguments.count == 2 { try prepared.write(to: URL(fileURLWithPath: CommandLine.arguments[1])) }
    print("PASS: \(count) privileged VPN config checks; no network changes")
  }
}

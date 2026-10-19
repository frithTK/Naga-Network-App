import Foundation

// A deliberately finite schema for privileged execution. Unknown fields fail
// closed, including new engine features, file paths, services and endpoints.
// Network credentials and inline TLS certificates stay in memory/root-owned
// session files; none are included in error messages or status reports.
enum VPNConfig {
  static func object(_ value: Any?, _ allowed: String) throws -> [String: Any] {
    guard let value = value as? [String: Any] else { throw VPNFailure("Некорректная структура конфигурации VPN") }
    let keys = Set(allowed.split(separator: " ").map(String.init))
    guard value.keys.allSatisfy({ keys.contains($0) }) else {
      throw VPNFailure("Профиль содержит параметры, пока не поддерживаемые VPN на macOS")
    }
    return value
  }

  static func scalars(_ value: Any, depth: Int = 0) throws {
    guard depth < 20 else { throw VPNFailure("Слишком сложная конфигурация VPN") }
    if let values = value as? [Any] { for item in values { try scalars(item, depth: depth + 1) }; return }
    guard value is String || value is NSNumber || value is NSNull else { throw VPNFailure("Недопустимый вложенный параметр VPN") }
  }

  static let dialKeys = "detour bind_interface inet4_bind_address inet6_bind_address connect_timeout tcp_fast_open tcp_multi_path udp_fragment domain_strategy network_strategy network_type fallback_network_type fallback_delay"
  static let ruleKeys = "type mode invert domain domain_suffix domain_keyword domain_regex ip_cidr source_ip_cidr ip_is_private source_ip_is_private port port_range source_port source_port_range network protocol inbound outbound action server strategy rule_set rule_set_ip_cidr_match_source rule_set_ip_cidr_accept_empty reject_method no_drop sniff timeout override_destination resolve rewrite_ttl client_subnet disable_cache disable_expire independent_cache auth_user clash_mode"

  static func rules(_ value: Any?, depth: Int = 0) throws {
    guard depth < 16 else { throw VPNFailure("Слишком глубокая вложенность правил VPN") }
    guard let list = value as? [Any], list.count <= 100000 else { throw VPNFailure("Некорректные правила VPN") }
    for item in list {
      let rule = try object(item, ruleKeys + " rules")
      for (key, value) in rule {
        if key == "rules" { try rules(value, depth: depth + 1) } else { try scalars(value) }
      }
    }
  }

  static func tls(_ raw: Any) throws {
    let value = try object(raw, "enabled disable_sni server_name insecure alpn min_version max_version cipher_suites certificate certificate_public_key_sha256 utls reality")
    for (key, item) in value {
      if key == "utls" { _ = try flat(item, "enabled fingerprint") }
      else if key == "reality" { _ = try flat(item, "enabled public_key short_id") }
      else { try scalars(item) }
    }
  }

  @discardableResult static func flat(_ raw: Any, _ keys: String) throws -> [String: Any] {
    let value = try object(raw, keys)
    for item in value.values { try scalars(item) }
    return value
  }

  static func outbound(_ raw: Any) throws {
    let value = try object(raw, "type tag server server_port server_ports uuid password method security alter_id flow packet_encoding network username version obfs obfs_param protocol protocol_param up_mbps down_mbps auth auth_str recv_window_conn recv_window disable_mtu_discovery zero_rtt_ handshake_timeout heartbeat congestion_control udp_relay_mode zero_rtt max_stream_receive_window max_connection_receive_window udp_over_stream brutal_debug outbounds default interrupt_exist_connections url interval tolerance idle_timeout tls transport multiplex udp_over_tcp domain_resolver " + dialKeys)
    let allowedTypes = ["direct", "block", "dns", "selector", "urltest", "vless", "vmess", "trojan", "shadowsocks", "hysteria", "hysteria2", "tuic", "socks", "http", "anytls"]
    guard let kind = value["type"] as? String, allowedTypes.contains(kind) else { throw VPNFailure("Этот протокол пока не поддерживается VPN на macOS") }
    for (key, item) in value {
      switch key {
      case "tls": try tls(item)
      case "transport":
        let transport = try object(item, "type host path headers max_early_data early_data_header_name service_name idle_timeout ping_timeout permit_without_stream method")
        guard let type = transport["type"] as? String, ["http", "ws", "grpc", "httpupgrade", "quic"].contains(type) else { throw VPNFailure("Неподдерживаемый транспорт VPN") }
        for (name, val) in transport {
          if name == "headers" {
            guard let headers = val as? [String: Any] else { throw VPNFailure("Некорректные заголовки транспорта") }
            for header in headers.values { try scalars(header) }
          } else { try scalars(val) }
        }
      case "multiplex":
        let mux = try object(item, "enabled protocol max_connections min_streams max_streams padding brutal")
        for (name, val) in mux {
          if name == "brutal" { try flat(val, "enabled up_mbps down_mbps") } else { try scalars(val) }
        }
      case "udp_over_tcp":
        if item is [String: Any] { try flat(item, "enabled version") } else { try scalars(item) }
      case "obfs":
        if item is [String: Any] { try flat(item, "type password") } else { try scalars(item) }
      case "domain_resolver":
        if item is [String: Any] { try flat(item, "server strategy disable_cache rewrite_ttl client_subnet") } else { try scalars(item) }
      default: try scalars(item)
      }
    }
  }

  static func prepare(_ data: Data, interface: String) throws -> Data {
    guard data.count <= vpnConfigLimit, interface.range(of: "^utun[0-9]{1,4}$", options: .regularExpression) != nil else { throw VPNFailure("Превышен размер конфигурации VPN") }
    let raw = try JSONSerialization.jsonObject(with: data, options: [.fragmentsAllowed])
    var config = try object(raw, "log dns inbounds outbounds route experimental")
    guard let outbounds = config["outbounds"] as? [Any], !outbounds.isEmpty, outbounds.count <= 2048 else { throw VPNFailure("В профиле нет VPN-серверов") }
    for value in outbounds { try outbound(value) }
    guard let inbounds = config["inbounds"] as? [[String: Any]], inbounds.count <= 8 else { throw VPNFailure("Не настроен VPN-интерфейс") }
    var owned: [[String: Any]] = []
    var tunCount = 0
    for value in inbounds {
      if value["type"] as? String == "tun" {
        tunCount += 1
        guard let tag = value["tag"] as? String else { throw VPNFailure("Не указан идентификатор VPN-интерфейса") }
        // Never accept routing, addresses, UIDs or interface names from a caller.
        owned.append(["type": "tun", "tag": tag, "interface_name": interface,
          "address": ["172.19.0.1/30", "fdfe:dcba:9876::1/126"],
          "mtu": 1500, "auto_route": false, "stack": "gvisor"])
      } else {
        let inbound = try flat(value, "type tag listen listen_port udp_timeout")
        guard inbound["type"] as? String == "mixed", inbound["listen"] as? String == "127.0.0.1",
          let port = inbound["listen_port"] as? Int, port >= 1024, port <= 65535 else { throw VPNFailure("VPN допускает только локальный proxy-порт") }
        owned.append(inbound)
      }
    }
    guard tunCount == 1 else { throw VPNFailure("Требуется ровно один VPN-интерфейс") }
    config["inbounds"] = owned
    if let raw = config["route"] {
      var route = try object(raw, "rules rule_set final auto_detect_interface default_domain_resolver default_interface find_process")
      if let rules = route["rules"] { try self.rules(rules) }
      if let sets = route["rule_set"] as? [Any] {
        for raw in sets {
          let set = try object(raw, "type tag rules")
          guard set["type"] as? String == "inline" else { throw VPNFailure("Правила VPN должны быть встроены в конфигурацию") }
          try rules(set["rules"])
        }
      } else if route["rule_set"] != nil { throw VPNFailure("Некорректный набор правил") }
      if let resolver = route["default_domain_resolver"] {
        if resolver is [String: Any] { try flat(resolver, "server strategy disable_cache rewrite_ttl client_subnet") } else { try scalars(resolver) }
      }
      route["auto_detect_interface"] = true
      route.removeValue(forKey: "default_interface")
      route["find_process"] = false
      config["route"] = route
    }
    if let raw = config["dns"] {
      let dns = try object(raw, "servers rules final strategy disable_cache disable_expire independent_cache reverse_mapping client_subnet cache_capacity")
      for (key, val) in dns {
        if key == "rules" { try rules(val) }
        else if key == "servers" {
          guard let servers = val as? [Any], servers.count <= 32 else { throw VPNFailure("Некорректные DNS-серверы") }
          for raw in servers {
            let server = try object(raw, "type tag server server_port path headers tls address address_resolver address_strategy strategy detour client_subnet domain_resolver " + dialKeys)
            guard let type = server["type"] as? String, ["udp", "tcp", "tls", "https", "quic", "h3", "local", "fakeip"].contains(type) else { throw VPNFailure("Неподдерживаемый DNS в профиле macOS") }
            for (name, item) in server {
              if name == "tls" { try tls(item) }
              else if name == "domain_resolver", item is [String: Any] { try flat(item, "server strategy disable_cache rewrite_ttl client_subnet") }
              else if name == "headers", let headers = item as? [String: Any] { for val in headers.values { try scalars(val) } }
              else { try scalars(item) }
            }
          }
        } else { try scalars(val) }
      }
    }
    let experimental = try object(config["experimental"], "clash_api")
    let api = try flat(experimental["clash_api"] as Any, "external_controller secret access_control_allow_origin access_control_allow_private_network")
    guard let address = api["external_controller"] as? String,
      address.range(of: "^127\\.0\\.0\\.1:[0-9]{4,5}$", options: .regularExpression) != nil,
      let port = Int(address.split(separator: ":").last!), (1024...65535).contains(port),
      let secret = api["secret"] as? String, secret.range(of: "^[0-9a-f]{32}$", options: .regularExpression) != nil else { throw VPNFailure("Некорректный локальный API VPN") }
    config["experimental"] = ["clash_api": ["external_controller": address, "secret": secret,
      "access_control_allow_origin": [String](), "access_control_allow_private_network": false]]
    config["log"] = ["level": "warn", "timestamp": true]
    return try JSONSerialization.data(withJSONObject: config, options: [.sortedKeys])
  }
}

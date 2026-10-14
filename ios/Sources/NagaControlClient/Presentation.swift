import Foundation

public extension ControlHealth {
    var ok: Bool { service == "naga-control" && status == "ok" }
}

public extension RuntimeSnapshot {
    var blocksProfileDelete: Bool { ["starting", "connected", "stopping"].contains(status) }
}

public extension StoredProfileSummary {
    var usedBytes: Int64 {
        let (sum, overflow) = max(0, uploadBytes).addingReportingOverflow(max(0, downloadBytes))
        return overflow ? Int64.max : sum
    }

    func isExpired(at now: Date = Date()) -> Bool {
        NagaFormat.date(expireUtc).map { $0 <= now } ?? false
    }

    func allowsConnection(at now: Date = Date()) -> Bool {
        canConnect && !isExpired(at: now) && (unlimited || totalBytes <= 0 || usedBytes < totalBytes)
    }
}

public extension UnifiedNode {
    var availableOnIOS: Bool { protocolName.caseInsensitiveCompare("olcRTC") != .orderedSame }
}

public enum NagaFormat {
    public static func bytes(_ value: Int64) -> String {
        let value = max(0, value)
        if value < 1_024 { return "\(value) Б" }
        if value < 1_048_576 { return String(format: "%.1f КБ", Double(value) / 1_024) }
        if value < 1_073_741_824 { return String(format: "%.1f МБ", Double(value) / 1_048_576) }
        return String(format: "%.2f ГБ", Double(value) / 1_073_741_824)
    }

    public static func date(_ value: String?) -> Date? {
        guard let value, !value.isEmpty else { return nil }
        let formatter = ISO8601DateFormatter()
        formatter.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
        if let date = formatter.date(from: value) { return date }
        formatter.formatOptions = [.withInternetDateTime]
        return formatter.date(from: value)
    }

    public static func session(_ startedAt: String?, now: Date = Date()) -> String {
        guard let date = date(startedAt) else { return "—" }
        let seconds = max(0, Int(now.timeIntervalSince(date)))
        if seconds >= 3_600 { return "\(seconds / 3_600) ч \(String(format: "%02d", seconds / 60 % 60)) мин" }
        return String(format: "%02d:%02d", seconds / 60, seconds % 60)
    }

    public static func expiry(_ value: String?) -> String {
        guard let date = date(value) else { return "Без срока действия" }
        let formatter = DateFormatter()
        formatter.locale = Locale(identifier: "ru_RU")
        formatter.dateFormat = "d MMM yyyy"
        return "До \(formatter.string(from: date))"
    }
}

public enum SubscriptionLink {
    public static func resolve(_ raw: String) -> String? {
        let raw = raw.trimmingCharacters(in: .whitespacesAndNewlines)
        guard let components = URLComponents(string: raw), let scheme = components.scheme?.lowercased() else { return nil }
        if scheme == "https", !(components.host ?? "").isEmpty { return raw }
        guard scheme == "nagavpn" else { return nil }
        let host = (components.host ?? "").lowercased()
        let path = components.path.trimmingCharacters(in: CharacterSet(charactersIn: "/")).lowercased()
        guard (host == "install-config" && (path.isEmpty || path == "install-config")) || (host.isEmpty && path == "install-config"),
              let inner = components.queryItems?.first(where: { $0.name == "url" })?.value,
              let url = URLComponents(string: inner), url.scheme?.lowercased() == "https", !(url.host ?? "").isEmpty else { return nil }
        return inner
    }
}

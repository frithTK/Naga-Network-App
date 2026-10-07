#include "flutter_window.h"

#include <optional>

#include <commdlg.h>
#include <shellapi.h>
#include <windows.h>
#include <flutter/standard_method_codec.h>

#include "flutter/generated_plugin_registrant.h"
#include "utils.h"
#include "window_constants.h"

namespace {

std::string StringArg(const flutter::EncodableMap* args, const char* key) {
  if (args == nullptr) {
    return {};
  }
  auto it = args->find(flutter::EncodableValue(key));
  if (it == args->end()) {
    return {};
  }
  if (const auto* value = std::get_if<std::string>(&it->second)) {
    return *value;
  }
  return {};
}

int64_t IntArg(const flutter::EncodableMap* args, const char* key) {
  if (args == nullptr) {
    return 0;
  }
  auto it = args->find(flutter::EncodableValue(key));
  if (it == args->end()) {
    return 0;
  }
  if (const auto* value = std::get_if<int32_t>(&it->second)) {
    return *value;
  }
  if (const auto* value = std::get_if<int64_t>(&it->second)) {
    return *value;
  }
  return 0;
}

bool StartDetachedQuoted(const std::wstring& command) {
  std::wstring writable = command;
  STARTUPINFOW si = {};
  si.cb = sizeof(si);
  PROCESS_INFORMATION pi = {};
  if (!CreateProcessW(nullptr, writable.data(), nullptr, nullptr, FALSE,
                      CREATE_NEW_PROCESS_GROUP | CREATE_UNICODE_ENVIRONMENT |
                          DETACHED_PROCESS,
                      nullptr, nullptr, &si, &pi)) {
    return false;
  }
  CloseHandle(pi.hThread);
  CloseHandle(pi.hProcess);
  return true;
}

}  // namespace

FlutterWindow::FlutterWindow(const flutter::DartProject& project)
    : project_(project) {}

FlutterWindow::~FlutterWindow() {}

bool FlutterWindow::OnCreate() {
  if (!Win32Window::OnCreate()) {
    return false;
  }

  RECT frame = GetClientArea();

  flutter_controller_ = std::make_unique<flutter::FlutterViewController>(
      frame.right - frame.left, frame.bottom - frame.top, project_);
  if (!flutter_controller_->engine() || !flutter_controller_->view()) {
    return false;
  }
  RegisterPlugins(flutter_controller_->engine());
  SetChildContent(flutter_controller_->view()->GetNativeWindow());
  SetupTrayChannel();
  SetupUpdateChannel();

  tray_.SetHost(GetHandle());
  tray_.SetActions(
      [this]() { ShowFromTray(); },
      [this]() { InvokeDart("toggleConnection"); },
      [this]() { InvokeDart("openServerPicker"); },
      [this]() { InvokeDart("quitApplication"); });
  tray_.Add();
  taskbar_created_ = RegisterWindowMessageW(L"TaskbarCreated");
  if (taskbar_created_ != 0) {
    ChangeWindowMessageFilterEx(GetHandle(), taskbar_created_, MSGFLT_ALLOW,
                                nullptr);
  }

  flutter_controller_->engine()->SetNextFrameCallback([this]() { this->Show(); });
  flutter_controller_->ForceRedraw();

  return true;
}

void FlutterWindow::SetupTrayChannel() {
  if (!flutter_controller_ || !flutter_controller_->engine()) {
    return;
  }
  tray_channel_ = std::make_unique<flutter::MethodChannel<flutter::EncodableValue>>(
      flutter_controller_->engine()->messenger(), "eu.nagavpn.naga_network/tray",
      &flutter::StandardMethodCodec::GetInstance());
  tray_channel_->SetMethodCallHandler(
      [this](const auto& call, auto result) {
        HandleTrayMethod(call, std::move(result));
      });
}

void FlutterWindow::InvokeDart(const char* method) {
  if (tray_channel_ == nullptr) {
    return;
  }
  tray_channel_->InvokeMethod(method, nullptr);
}

void FlutterWindow::InvokeDart(const char* method,
                               const flutter::EncodableValue& args) {
  if (tray_channel_ == nullptr) {
    return;
  }
  tray_channel_->InvokeMethod(
      method, std::make_unique<flutter::EncodableValue>(args));
}

void FlutterWindow::HandleInstallConfig(const std::string& uri) {
  if (uri.empty()) {
    return;
  }
  InvokeDart("installConfig", flutter::EncodableValue(uri));
}

void FlutterWindow::SetupUpdateChannel() {
  if (!flutter_controller_ || !flutter_controller_->engine()) {
    return;
  }
  update_channel_ =
      std::make_unique<flutter::MethodChannel<flutter::EncodableValue>>(
          flutter_controller_->engine()->messenger(),
          "eu.nagavpn.naga_network/update",
          &flutter::StandardMethodCodec::GetInstance());
  update_channel_->SetMethodCallHandler(
      [this](const auto& call, auto result) {
        const std::string& method = call.method_name();
        const auto* args = std::get_if<flutter::EncodableMap>(call.arguments());
        if (method == "applyWindowsPortable") {
          const std::wstring helper = Utf16FromUtf8(StringArg(args, "helperPath"));
          const std::wstring zip = Utf16FromUtf8(StringArg(args, "zipPath"));
          const std::wstring dest = Utf16FromUtf8(StringArg(args, "installDir"));
          std::wstring launch = Utf16FromUtf8(StringArg(args, "launch"));
          if (launch.empty()) {
            launch = L"NagaNetwork.exe";
          }
          int64_t pid = IntArg(args, "pid");
          if (pid <= 0) {
            pid = GetCurrentProcessId();
          }
          const std::wstring command =
              L"\"" + helper + L"\" --wait-pid " + std::to_wstring(pid) +
              L" --zip \"" + zip + L"\" --to \"" + dest + L"\" --launch \"" +
              launch + L"\"";
          if (!StartDetachedQuoted(command)) {
            result->Error("spawn", "failed to start naga-update.exe");
            return;
          }
          result->Success();
          tray_.Remove();
          QuitApplication();
          return;
        }
        if (method == "applyWindowsSetup") {
          const std::wstring setup = Utf16FromUtf8(StringArg(args, "setupPath"));
          if (setup.empty() ||
              reinterpret_cast<INT_PTR>(ShellExecuteW(
                  GetHandle(), L"open", setup.c_str(), nullptr, nullptr,
                  SW_SHOWNORMAL)) <= 32) {
            result->Error("spawn", "failed to start setup.exe");
            return;
          }
          result->Success();
          return;
        }
        result->NotImplemented();
      });
}

void FlutterWindow::HandleTrayMethod(
    const flutter::MethodCall<flutter::EncodableValue>& call,
    std::unique_ptr<flutter::MethodResult<flutter::EncodableValue>> result) {
  const std::string& method = call.method_name();
  if (method == "updateStatus") {
    const auto* args = std::get_if<flutter::EncodableMap>(call.arguments());
    tray_.UpdateStatus(StringArg(args, "status"), StringArg(args, "profile"),
                       StringArg(args, "server"));
    result->Success();
    return;
  }
  if (method == "terminate") {
    result->Success();
    tray_.Remove();
    QuitApplication();
    return;
  }
  if (method == "minimize") {
    if (HWND hwnd = GetHandle()) {
      ShowWindow(hwnd, SW_MINIMIZE);
    }
    result->Success();
    return;
  }
  if (method == "maximize") {
    ToggleMaximize();
    result->Success();
    return;
  }
  if (method == "close") {
    HideToTray();
    result->Success();
    return;
  }
  if (method == "startDrag") {
    HWND hwnd = GetHandle();
    result->Success();
    if (hwnd != nullptr) {
      ReleaseCapture();
      SendMessage(hwnd, WM_SYSCOMMAND, SC_MOVE | HTCAPTION, 0);
    }
    return;
  }
  if (method == "pickConfigFile") {
    wchar_t path[MAX_PATH] = {};
    OPENFILENAMEW ofn = {};
    ofn.lStructSize = sizeof(ofn);
    ofn.hwndOwner = GetHandle();
    ofn.lpstrFilter =
        L"VPN config (*.json;*.conf;*.txt)\0*.json;*.conf;*.txt\0All files "
        L"(*.*)\0*.*\0";
    ofn.lpstrFile = path;
    ofn.nMaxFile = MAX_PATH;
    ofn.lpstrTitle = L"Naga Network";
    ofn.Flags = OFN_FILEMUSTEXIST | OFN_PATHMUSTEXIST | OFN_EXPLORER |
                OFN_NOCHANGEDIR;
    if (!GetOpenFileNameW(&ofn)) {
      if (CommDlgExtendedError() == 0) {
        result->Success();
        return;
      }
      result->Error("file_dialog", "windows file dialog failed");
      return;
    }
    result->Success(flutter::EncodableValue(Utf8FromUtf16(path)));
    return;
  }
  result->NotImplemented();
}

void FlutterWindow::ToggleMaximize() {
  HWND hwnd = GetHandle();
  if (hwnd == nullptr) {
    return;
  }
  if (IsZoomed(hwnd)) {
    ShowWindow(hwnd, SW_RESTORE);
  } else {
    ShowWindow(hwnd, SW_MAXIMIZE);
  }
}

void FlutterWindow::OnDestroy() {
  tray_.Remove();
  tray_channel_.reset();
  if (flutter_controller_) {
    flutter_controller_ = nullptr;
  }

  Win32Window::OnDestroy();
}

void FlutterWindow::OnHiddenToTray() { tray_.MaybeShowBackgroundHint(); }

LRESULT
FlutterWindow::MessageHandler(HWND hwnd, UINT const message,
                              WPARAM const wparam,
                              LPARAM const lparam) noexcept {
  if (!is_quitting()) {
    if (message == WM_CLOSE) {
      HideToTray();
      return 0;
    }
    if (message == WM_SYSCOMMAND && (wparam & 0xFFF0) == SC_CLOSE) {
      HideToTray();
      return 0;
    }
  }
  if (message == kNagaShowFromOtherProcess) {
    ShowFromTray();
    return 0;
  }
  if (message == WM_COPYDATA) {
    auto* data = reinterpret_cast<COPYDATASTRUCT*>(lparam);
    if (data != nullptr && data->dwData == kNagaCopyInstallConfig &&
        data->lpData != nullptr && data->cbData > 0) {
      const char* bytes = static_cast<const char*>(data->lpData);
      std::string uri(bytes, data->cbData);
      if (!uri.empty() && uri.back() == '\0') {
        uri.pop_back();
      }
      HandleInstallConfig(uri);
      ShowFromTray();
    }
    return 0;
  }
  if (taskbar_created_ != 0 && message == taskbar_created_) {
    tray_.ReAddAfterTaskbarCreated();
    return 0;
  }
  if (tray_.HandleWindowMessage(message, wparam, lparam)) {
    return 0;
  }
  if (message == WM_NCCALCSIZE || message == WM_NCHITTEST ||
      message == WM_GETMINMAXINFO || message == WM_NCACTIVATE ||
      message == WM_NCLBUTTONDOWN || message == WM_NCLBUTTONDBLCLK) {
    return Win32Window::MessageHandler(hwnd, message, wparam, lparam);
  }

  if (flutter_controller_) {
    std::optional<LRESULT> result =
        flutter_controller_->HandleTopLevelWindowProc(hwnd, message, wparam,
                                                      lparam);
    if (result) {
      return *result;
    }
  }

  switch (message) {
    case WM_FONTCHANGE:
      if (flutter_controller_) {
        flutter_controller_->engine()->ReloadSystemFonts();
      }
      break;
  }

  return Win32Window::MessageHandler(hwnd, message, wparam, lparam);
}

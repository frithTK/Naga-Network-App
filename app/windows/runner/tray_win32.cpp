#include "tray_win32.h"

#include <shellapi.h>

#include "resource.h"
#include "utils.h"
#include "window_constants.h"

namespace {

constexpr UINT kTrayIconId = 1;
constexpr UINT kCmdToggle = 40001;
constexpr UINT kCmdServers = 40002;
constexpr UINT kCmdOpen = 40003;
constexpr UINT kCmdQuit = 40004;

std::wstring LoadStatusLabel(const std::string& status) {
  if (status == "connected") {
    return L"● Соединение защищено";
  }
  if (status == "connecting") {
    return L"◌ Подключение…";
  }
  if (status == "error") {
    return L"! Ошибка подключения";
  }
  return L"○ VPN отключён";
}

}  // namespace

TrayWin32::TrayWin32() = default;

TrayWin32::~TrayWin32() {
  Remove();
  if (small_icon_ != nullptr) {
    DestroyIcon(small_icon_);
    small_icon_ = nullptr;
  }
}

void TrayWin32::SetHost(HWND hwnd) { hwnd_ = hwnd; }

void TrayWin32::SetActions(std::function<void()> on_show,
                           std::function<void()> on_toggle,
                           std::function<void()> on_servers,
                           std::function<void()> on_quit) {
  on_show_ = std::move(on_show);
  on_toggle_ = std::move(on_toggle);
  on_servers_ = std::move(on_servers);
  on_quit_ = std::move(on_quit);
}

NOTIFYICONDATAW TrayWin32::BaseNid() const {
  NOTIFYICONDATAW nid{};
  nid.cbSize = sizeof(nid);
  nid.hWnd = hwnd_;
  nid.uID = kTrayIconId;
  nid.uFlags = NIF_MESSAGE | NIF_ICON | NIF_TIP;
  nid.uCallbackMessage = kNagaTrayCallback;
  nid.hIcon = small_icon_;
  wcsncpy_s(nid.szTip, tooltip_.c_str(), _TRUNCATE);
  return nid;
}

bool TrayWin32::Add() {
  if (hwnd_ == nullptr || added_) {
    return added_;
  }
  if (small_icon_ == nullptr) {
    small_icon_ = static_cast<HICON>(LoadImageW(
        GetModuleHandle(nullptr), MAKEINTRESOURCE(IDI_APP_ICON), IMAGE_ICON, 16,
        16, LR_DEFAULTCOLOR));
  }
  NOTIFYICONDATAW nid = BaseNid();
  if (!Shell_NotifyIconW(NIM_ADD, &nid)) {
    return false;
  }
  nid.uVersion = NOTIFYICON_VERSION_4;
  Shell_NotifyIconW(NIM_SETVERSION, &nid);
  added_ = true;
  return true;
}

void TrayWin32::Remove() {
  if (!added_ || hwnd_ == nullptr) {
    added_ = false;
    return;
  }
  NOTIFYICONDATAW nid = BaseNid();
  Shell_NotifyIconW(NIM_DELETE, &nid);
  added_ = false;
}

void TrayWin32::ReAddAfterTaskbarCreated() {
  added_ = false;
  Add();
}

void TrayWin32::UpdateStatus(const std::string& status,
                             const std::string& profile,
                             const std::string& server) {
  status_code_ = status.empty() ? "disconnected" : status;
  status_label_ = LoadStatusLabel(status_code_);
  profile_label_ = profile.empty() ? L"Профиль не выбран" : Utf16FromUtf8(profile);
  if (server.empty()) {
    tooltip_ = L"Naga Network — " + status_label_;
  } else {
    tooltip_ = L"Naga Network — " + Utf16FromUtf8(server);
  }
  if (!added_) {
    return;
  }
  NOTIFYICONDATAW nid = BaseNid();
  Shell_NotifyIconW(NIM_MODIFY, &nid);
}

void TrayWin32::MaybeShowBackgroundHint() {
  if (hint_checked_) {
    return;
  }
  hint_checked_ = true;
  const std::wstring dir = DataDir();
  const std::wstring flag = dir + L"\\tray-hint-shown";
  if (GetFileAttributesW(flag.c_str()) != INVALID_FILE_ATTRIBUTES) {
    return;
  }
  if (added_) {
    NOTIFYICONDATAW nid = BaseNid();
    nid.uFlags |= NIF_INFO;
    wcsncpy_s(nid.szInfoTitle, L"Naga Network", _TRUNCATE);
    wcsncpy_s(nid.szInfo, L"Naga продолжает работать в фоне.", _TRUNCATE);
    nid.dwInfoFlags = NIIF_INFO | NIIF_NOSOUND;
    Shell_NotifyIconW(NIM_MODIFY, &nid);
  }
  CreateDirectoryW(dir.c_str(), nullptr);
  HANDLE file = CreateFileW(flag.c_str(), GENERIC_WRITE, 0, nullptr,
                            CREATE_NEW, FILE_ATTRIBUTE_NORMAL, nullptr);
  if (file != INVALID_HANDLE_VALUE) {
    CloseHandle(file);
  }
}

bool TrayWin32::HandleWindowMessage(UINT message, WPARAM wparam,
                                    LPARAM lparam) {
  if (message == kNagaTrayCallback) {
    const UINT event = LOWORD(lparam);
    switch (event) {
      case WM_LBUTTONUP:
      case NIN_SELECT:
      case NIN_KEYSELECT:
        if (on_show_) {
          on_show_();
        }
        return true;
      case WM_CONTEXTMENU:
      case WM_RBUTTONUP:
        ShowMenu();
        return true;
      default:
        return true;
    }
  }
  if (message == WM_COMMAND) {
    switch (LOWORD(wparam)) {
      case kCmdOpen:
        if (on_show_) {
          on_show_();
        }
        return true;
      case kCmdToggle:
        if (on_toggle_) {
          on_toggle_();
        }
        return true;
      case kCmdServers:
        if (on_show_) {
          on_show_();
        }
        if (on_servers_) {
          on_servers_();
        }
        return true;
      case kCmdQuit:
        if (on_quit_) {
          on_quit_();
        }
        return true;
      default:
        break;
    }
  }
  return false;
}

void TrayWin32::ShowMenu() const {
  if (hwnd_ == nullptr) {
    return;
  }
  POINT cursor{};
  GetCursorPos(&cursor);
  HMENU menu = CreatePopupMenu();
  if (menu == nullptr) {
    return;
  }
  AppendMenuW(menu, MF_STRING | MF_GRAYED, 0, L"Naga Network");
  AppendMenuW(menu, MF_STRING | MF_GRAYED, 0, status_label_.c_str());
  AppendMenuW(menu, MF_STRING | MF_GRAYED, 0, profile_label_.c_str());
  AppendMenuW(menu, MF_SEPARATOR, 0, nullptr);
  const bool connected = status_code_ == "connected";
  AppendMenuW(menu, MF_STRING, kCmdToggle,
              connected ? L"Отключиться" : L"Подключиться");
  AppendMenuW(menu, MF_STRING, kCmdServers, L"Сменить сервер");
  AppendMenuW(menu, MF_STRING, kCmdOpen, L"Открыть Naga Network");
  AppendMenuW(menu, MF_SEPARATOR, 0, nullptr);
  AppendMenuW(menu, MF_STRING, kCmdQuit, L"Выход");
  SetForegroundWindow(hwnd_);
  TrackPopupMenu(menu, TPM_RIGHTBUTTON | TPM_BOTTOMALIGN, cursor.x, cursor.y, 0,
                 hwnd_, nullptr);
  PostMessageW(hwnd_, WM_NULL, 0, 0);
  DestroyMenu(menu);
}

std::wstring TrayWin32::DataDir() const {
  wchar_t buffer[MAX_PATH] = {};
  DWORD length = GetEnvironmentVariableW(L"NAGA_DATA_DIR", buffer, MAX_PATH);
  if (length > 0 && length < MAX_PATH) {
    return buffer;
  }
  length = GetEnvironmentVariableW(L"LOCALAPPDATA", buffer, MAX_PATH);
  if (length > 0 && length < MAX_PATH) {
    return std::wstring(buffer) + L"\\naga-network";
  }
  return L".";
}

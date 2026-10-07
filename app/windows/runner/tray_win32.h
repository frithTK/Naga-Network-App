#ifndef RUNNER_TRAY_WIN32_H_
#define RUNNER_TRAY_WIN32_H_

#include <windows.h>

#include <functional>
#include <string>

class TrayWin32 {
 public:
  TrayWin32();
  ~TrayWin32();

  TrayWin32(const TrayWin32&) = delete;
  TrayWin32& operator=(const TrayWin32&) = delete;

  void SetHost(HWND hwnd);
  void SetActions(std::function<void()> on_show,
                  std::function<void()> on_toggle,
                  std::function<void()> on_servers,
                  std::function<void()> on_quit);

  bool Add();
  void Remove();
  void ReAddAfterTaskbarCreated();
  void UpdateStatus(const std::string& status,
                    const std::string& profile,
                    const std::string& server);
  void MaybeShowBackgroundHint();
  bool HandleWindowMessage(UINT message, WPARAM wparam, LPARAM lparam);

 private:
  void ShowMenu() const;
  NOTIFYICONDATAW BaseNid() const;
  std::wstring DataDir() const;

  HWND hwnd_ = nullptr;
  bool added_ = false;
  bool hint_checked_ = false;
  HICON small_icon_ = nullptr;
  std::string status_code_ = "disconnected";
  std::wstring status_label_ = L"○ VPN отключён";
  std::wstring profile_label_ = L"Профиль не выбран";
  std::wstring tooltip_ = L"Naga Network";
  std::function<void()> on_show_;
  std::function<void()> on_toggle_;
  std::function<void()> on_servers_;
  std::function<void()> on_quit_;
};

#endif  // RUNNER_TRAY_WIN32_H_

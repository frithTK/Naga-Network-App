#ifndef RUNNER_FLUTTER_WINDOW_H_
#define RUNNER_FLUTTER_WINDOW_H_

#include <flutter/dart_project.h>
#include <flutter/flutter_view_controller.h>
#include <flutter/method_channel.h>
#include <flutter/encodable_value.h>

#include <memory>
#include <string>

#include "tray_win32.h"
#include "win32_window.h"

class FlutterWindow : public Win32Window {
 public:
  explicit FlutterWindow(const flutter::DartProject& project);
  virtual ~FlutterWindow();

 protected:
  bool OnCreate() override;
  void OnDestroy() override;
  void OnHiddenToTray() override;
  LRESULT MessageHandler(HWND window, UINT const message, WPARAM const wparam,
                         LPARAM const lparam) noexcept override;

 private:
  void SetupTrayChannel();
  void SetupUpdateChannel();
  void InvokeDart(const char* method);
  void InvokeDart(const char* method, const flutter::EncodableValue& args);
  void HandleInstallConfig(const std::string& uri);
  void HandleTrayMethod(
      const flutter::MethodCall<flutter::EncodableValue>& call,
      std::unique_ptr<flutter::MethodResult<flutter::EncodableValue>> result);
  void ToggleMaximize();

  flutter::DartProject project_;
  std::unique_ptr<flutter::FlutterViewController> flutter_controller_;
  std::unique_ptr<flutter::MethodChannel<flutter::EncodableValue>> tray_channel_;
  std::unique_ptr<flutter::MethodChannel<flutter::EncodableValue>>
      update_channel_;
  TrayWin32 tray_;
  UINT taskbar_created_ = 0;
};

#endif  // RUNNER_FLUTTER_WINDOW_H_

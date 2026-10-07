#include <flutter/dart_project.h>
#include <flutter/flutter_view_controller.h>
#include <windows.h>
#include <shobjidl.h>

#include <string>
#include <vector>

#include "flutter_window.h"
#include "utils.h"
#include "window_constants.h"

namespace {

std::string FirstInstallArgument(const std::vector<std::string>& args) {
  for (auto it = args.rbegin(); it != args.rend(); ++it) {
    if (it->rfind("nagavpn:", 0) == 0 || it->rfind("https:", 0) == 0) {
      return *it;
    }
  }
  return {};
}

bool ForwardInstallConfig(HWND hwnd, const std::string& uri) {
  if (hwnd == nullptr || uri.empty()) {
    return false;
  }
  COPYDATASTRUCT data{};
  data.dwData = kNagaCopyInstallConfig;
  data.cbData = static_cast<DWORD>(uri.size());
  data.lpData = const_cast<char*>(uri.data());
  SendMessageW(hwnd, WM_COPYDATA, 0, reinterpret_cast<LPARAM>(&data));
  return true;
}

bool ActivateExistingInstance(const std::string& uri) {
  HANDLE ready = CreateEventW(nullptr, TRUE, FALSE, kNagaReadyEventName);
  if (ready == nullptr) {
    return false;
  }
  const DWORD wait = WaitForSingleObject(ready, 15000);
  CloseHandle(ready);
  if (wait != WAIT_OBJECT_0) {
    return false;
  }
  HWND hwnd = FindWindowW(kNagaWindowClass, nullptr);
  if (hwnd == nullptr) {
    return false;
  }
  DWORD pid = 0;
  GetWindowThreadProcessId(hwnd, &pid);
  if (pid != 0) {
    AllowSetForegroundWindow(pid);
  }
  ForwardInstallConfig(hwnd, uri);
  PostMessageW(hwnd, kNagaShowFromOtherProcess, 0, 0);
  return true;
}

}  // namespace

int APIENTRY wWinMain(_In_ HINSTANCE instance, _In_opt_ HINSTANCE prev,
                      _In_ wchar_t* command_line, _In_ int show_command) {
  if (!::AttachConsole(ATTACH_PARENT_PROCESS) && ::IsDebuggerPresent()) {
    CreateAndAttachConsole();
  }

  ::CoInitializeEx(nullptr, COINIT_APARTMENTTHREADED);
  SetCurrentProcessExplicitAppUserModelID(kNagaAppUserModelId);

  HANDLE mutex = CreateMutexW(nullptr, TRUE, kNagaMutexName);
  if (mutex == nullptr) {
    ::CoUninitialize();
    return EXIT_FAILURE;
  }
  const DWORD mutex_error = GetLastError();
  std::vector<std::string> command_line_arguments = GetCommandLineArguments();
  if (mutex_error == ERROR_ALREADY_EXISTS) {
    ActivateExistingInstance(FirstInstallArgument(command_line_arguments));
    CloseHandle(mutex);
    ::CoUninitialize();
    return EXIT_SUCCESS;
  }

  HANDLE ready = CreateEventW(nullptr, TRUE, FALSE, kNagaReadyEventName);

  flutter::DartProject project(L"data");
  project.set_dart_entrypoint_arguments(std::move(command_line_arguments));

  FlutterWindow window(project);
  Win32Window::Point origin(10, 10);
  Win32Window::Size size(1280, 720);
  if (!window.Create(L"Naga Network", origin, size)) {
    if (ready != nullptr) {
      CloseHandle(ready);
    }
    CloseHandle(mutex);
    ::CoUninitialize();
    return EXIT_FAILURE;
  }
  window.SetQuitOnClose(false);
  if (ready != nullptr) {
    SetEvent(ready);
  }

  ::MSG msg;
  while (::GetMessage(&msg, nullptr, 0, 0)) {
    ::TranslateMessage(&msg);
    ::DispatchMessage(&msg);
  }

  if (ready != nullptr) {
    CloseHandle(ready);
  }
  CloseHandle(mutex);
  ::CoUninitialize();
  return EXIT_SUCCESS;
}

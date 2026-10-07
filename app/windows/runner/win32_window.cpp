#include "win32_window.h"

#include <algorithm>
#include <dwmapi.h>
#include <flutter_windows.h>
#include <windowsx.h>

#include "resource.h"
#include "window_constants.h"

namespace {

#ifndef DWMWA_USE_IMMERSIVE_DARK_MODE
#define DWMWA_USE_IMMERSIVE_DARK_MODE 20
#endif

constexpr wchar_t kGetPreferredBrightnessRegKey[] =
    L"Software\\Microsoft\\Windows\\CurrentVersion\\Themes\\Personalize";
constexpr wchar_t kGetPreferredBrightnessRegValue[] = L"AppsUseLightTheme";

static int g_active_window_count = 0;

int Scale(int source, double scale_factor) {
  return static_cast<int>(source * scale_factor);
}

UINT WindowDpi(HWND hwnd) {
  using GetDpiForWindowFn = UINT(WINAPI*)(HWND);
  static GetDpiForWindowFn get_dpi = reinterpret_cast<GetDpiForWindowFn>(
      GetProcAddress(GetModuleHandleW(L"user32.dll"), "GetDpiForWindow"));
  if (get_dpi != nullptr && hwnd != nullptr) {
    UINT dpi = get_dpi(hwnd);
    if (dpi != 0) {
      return dpi;
    }
  }
  return 96;
}

int FrameThickness(HWND hwnd) {
  const UINT dpi = WindowDpi(hwnd);
  using GetSystemMetricsForDpiFn = int(WINAPI*)(int, UINT);
  static GetSystemMetricsForDpiFn metrics =
      reinterpret_cast<GetSystemMetricsForDpiFn>(GetProcAddress(
          GetModuleHandleW(L"user32.dll"), "GetSystemMetricsForDpi"));
  if (metrics != nullptr) {
    return metrics(SM_CXFRAME, dpi) + metrics(SM_CXPADDEDBORDER, dpi);
  }
  return GetSystemMetrics(SM_CXFRAME) + GetSystemMetrics(SM_CXPADDEDBORDER);
}

}  // namespace

class WindowClassRegistrar {
 public:
  ~WindowClassRegistrar() = default;

  static WindowClassRegistrar* GetInstance() {
    if (!instance_) {
      instance_ = new WindowClassRegistrar();
    }
    return instance_;
  }

  const wchar_t* GetWindowClass();
  void UnregisterWindowClass();

 private:
  WindowClassRegistrar() = default;

  static WindowClassRegistrar* instance_;
  bool class_registered_ = false;
};

WindowClassRegistrar* WindowClassRegistrar::instance_ = nullptr;

const wchar_t* WindowClassRegistrar::GetWindowClass() {
  if (!class_registered_) {
    WNDCLASSEXW window_class{};
    window_class.cbSize = sizeof(window_class);
    window_class.hCursor = LoadCursor(nullptr, IDC_ARROW);
    window_class.lpszClassName = kNagaWindowClass;
    window_class.style = CS_HREDRAW | CS_VREDRAW;
    window_class.cbClsExtra = 0;
    window_class.cbWndExtra = 0;
    window_class.hInstance = GetModuleHandle(nullptr);
    window_class.hIcon =
        LoadIcon(window_class.hInstance, MAKEINTRESOURCE(IDI_APP_ICON));
    window_class.hIconSm = static_cast<HICON>(LoadImageW(
        window_class.hInstance, MAKEINTRESOURCE(IDI_APP_ICON), IMAGE_ICON,
        GetSystemMetrics(SM_CXSMICON), GetSystemMetrics(SM_CYSMICON),
        LR_DEFAULTCOLOR));
    window_class.hbrBackground = 0;
    window_class.lpszMenuName = nullptr;
    window_class.lpfnWndProc = Win32Window::WndProc;
    RegisterClassExW(&window_class);
    class_registered_ = true;
  }
  return kNagaWindowClass;
}

void WindowClassRegistrar::UnregisterWindowClass() {
  UnregisterClass(kNagaWindowClass, nullptr);
  class_registered_ = false;
}

Win32Window::Win32Window() { ++g_active_window_count; }

Win32Window::~Win32Window() {
  --g_active_window_count;
  Destroy();
}

bool Win32Window::Create(const std::wstring& title, const Point& origin,
                         const Size& size) {
  Destroy();

  const wchar_t* window_class =
      WindowClassRegistrar::GetInstance()->GetWindowClass();

  const POINT target_point = {static_cast<LONG>(origin.x),
                              static_cast<LONG>(origin.y)};
  HMONITOR monitor = MonitorFromPoint(target_point, MONITOR_DEFAULTTONEAREST);
  UINT dpi = FlutterDesktopGetDpiForMonitor(monitor);
  double scale_factor = dpi / 96.0;

  HWND window = CreateWindow(
      window_class, title.c_str(), WS_OVERLAPPEDWINDOW,
      Scale(origin.x, scale_factor), Scale(origin.y, scale_factor),
      Scale(size.width, scale_factor), Scale(size.height, scale_factor),
      nullptr, nullptr, GetModuleHandle(nullptr), this);

  if (!window) {
    return false;
  }

  UpdateTheme(window);
  ApplyCustomFrame();

  if (!OnCreate()) {
    return false;
  }
  ApplyCustomFrame();
  return true;
}

bool Win32Window::Show() {
  ApplyCustomFrame();
  return ShowWindow(window_handle_, SW_SHOWNORMAL);
}

void Win32Window::ApplyCustomFrame() {
  if (window_handle_ == nullptr) {
    return;
  }
  // 1 px into the client hides the Win10 white caption strip.
  MARGINS margins{0, 0, 1, 0};
  DwmExtendFrameIntoClientArea(window_handle_, &margins);
  SetWindowPos(window_handle_, nullptr, 0, 0, 0, 0,
               SWP_FRAMECHANGED | SWP_NOMOVE | SWP_NOSIZE | SWP_NOZORDER |
                   SWP_NOOWNERZORDER | SWP_NOACTIVATE);
}

void Win32Window::HideToTray() {
  if (window_handle_ == nullptr || quitting_) {
    return;
  }
  ShowWindow(window_handle_, SW_HIDE);
  OnHiddenToTray();
}

void Win32Window::ShowFromTray() {
  if (window_handle_ == nullptr) {
    return;
  }
  if (IsIconic(window_handle_)) {
    ShowWindow(window_handle_, SW_RESTORE);
  } else {
    ShowWindow(window_handle_, SW_SHOW);
  }
  SetForegroundWindow(window_handle_);
  BringWindowToTop(window_handle_);
}

void Win32Window::QuitApplication() {
  quitting_ = true;
  HWND hwnd = window_handle_;
  if (hwnd != nullptr) {
    DestroyWindow(hwnd);
  }
  PostQuitMessage(0);
}

void Win32Window::OnHiddenToTray() {}

LRESULT CALLBACK Win32Window::WndProc(HWND const window, UINT const message,
                                      WPARAM const wparam,
                                      LPARAM const lparam) noexcept {
  if (message == WM_NCCREATE) {
    auto window_struct = reinterpret_cast<CREATESTRUCT*>(lparam);
    SetWindowLongPtr(window, GWLP_USERDATA,
                     reinterpret_cast<LONG_PTR>(window_struct->lpCreateParams));

    auto that = static_cast<Win32Window*>(window_struct->lpCreateParams);
    that->window_handle_ = window;
  }

  if (Win32Window* that = GetThisFromHandle(window)) {
    // Nested WM_NCCALCSIZE during WM_NCCREATE must not fall through to
    // DefWindowProc, or Windows keeps the stock caption forever.
    if (message == WM_NCCALCSIZE) {
      return that->HandleNcCalcSize(window, wparam, lparam);
    }
    if (message != WM_NCCREATE) {
      return that->MessageHandler(window, message, wparam, lparam);
    }
  }

  return DefWindowProc(window, message, wparam, lparam);
}

LRESULT Win32Window::HandleNcCalcSize(HWND hwnd, WPARAM wparam,
                                      LPARAM lparam) const {
  const int frame = FrameThickness(hwnd);
  RECT* client = nullptr;
  if (wparam) {
    client = &(reinterpret_cast<NCCALCSIZE_PARAMS*>(lparam)->rgrc[0]);
  } else {
    client = reinterpret_cast<RECT*>(lparam);
  }
  if (IsZoomed(hwnd)) {
    MONITORINFO monitor_info{};
    monitor_info.cbSize = sizeof(monitor_info);
    if (GetMonitorInfoW(MonitorFromWindow(hwnd, MONITOR_DEFAULTTONEAREST),
                        &monitor_info)) {
      *client = monitor_info.rcWork;
      return 0;
    }
  }
  client->left += frame;
  client->right -= frame;
  client->bottom -= frame;
  return 0;
}

LRESULT Win32Window::HandleNcHitTest(HWND hwnd, LPARAM lparam) const {
  POINT point{GET_X_LPARAM(lparam), GET_Y_LPARAM(lparam)};
  ScreenToClient(hwnd, &point);
  RECT client{};
  GetClientRect(hwnd, &client);
  const int frame = FrameThickness(hwnd);
  const UINT dpi = WindowDpi(hwnd);
  const int title_height = MulDiv(kNagaTitleBarHeight, static_cast<int>(dpi), 96);
  const int button_span =
      MulDiv(kNagaCaptionButtonSpan, static_cast<int>(dpi), 96);
  const int resize =
      (std::min)(frame, MulDiv(6, static_cast<int>(dpi), 96));
  const bool maximized = IsZoomed(hwnd) != FALSE;

  if (!maximized) {
    const bool top = point.y < resize;
    const bool bottom = point.y >= client.bottom - resize;
    const bool left = point.x < resize;
    const bool right = point.x >= client.right - resize;
    if (top && left) {
      return HTTOPLEFT;
    }
    if (top && right) {
      return HTTOPRIGHT;
    }
    if (bottom && left) {
      return HTBOTTOMLEFT;
    }
    if (bottom && right) {
      return HTBOTTOMRIGHT;
    }
    if (top) {
      return HTTOP;
    }
    if (bottom) {
      return HTBOTTOM;
    }
    if (left) {
      return HTLEFT;
    }
    if (right) {
      return HTRIGHT;
    }
  }

  if (point.y >= 0 && point.y < title_height) {
    if (point.x > client.right - button_span) {
      return HTCLIENT;
    }
    return HTCAPTION;
  }
  return HTCLIENT;
}

void Win32Window::HandleMinMaxInfo(HWND hwnd, LPARAM lparam) const {
  auto* info = reinterpret_cast<MINMAXINFO*>(lparam);
  MONITORINFO monitor_info{};
  monitor_info.cbSize = sizeof(monitor_info);
  if (!GetMonitorInfoW(MonitorFromWindow(hwnd, MONITOR_DEFAULTTONEAREST),
                       &monitor_info)) {
    return;
  }
  const RECT& work = monitor_info.rcWork;
  const RECT& monitor = monitor_info.rcMonitor;
  info->ptMaxPosition.x = work.left - monitor.left;
  info->ptMaxPosition.y = work.top - monitor.top;
  info->ptMaxSize.x = work.right - work.left;
  info->ptMaxSize.y = work.bottom - work.top;

  const UINT dpi = WindowDpi(hwnd);
  const int frame = FrameThickness(hwnd);
  const int min_outer_w =
      MulDiv(kNagaMinWindowWidth, static_cast<int>(dpi), 96) + (2 * frame);
  const int min_outer_h =
      MulDiv(kNagaMinWindowHeight, static_cast<int>(dpi), 96) + frame;
  const int work_w = work.right - work.left;
  const int work_h = work.bottom - work.top;
  info->ptMinTrackSize.x = (std::min)(min_outer_w, work_w);
  info->ptMinTrackSize.y = (std::min)(min_outer_h, work_h);
}

LRESULT
Win32Window::MessageHandler(HWND hwnd, UINT const message, WPARAM const wparam,
                            LPARAM const lparam) noexcept {
  switch (message) {
    case WM_NCCALCSIZE:
      return HandleNcCalcSize(hwnd, wparam, lparam);
    case WM_NCACTIVATE:
      // lParam -1: do not paint the stock caption/buttons.
      return DefWindowProc(hwnd, WM_NCACTIVATE, wparam, static_cast<LPARAM>(-1));
    case WM_ACTIVATE: {
      MARGINS margins{0, 0, 1, 0};
      DwmExtendFrameIntoClientArea(hwnd, &margins);
      if (child_content_ != nullptr) {
        SetFocus(child_content_);
      }
      return 0;
    }
    case WM_NCHITTEST: {
      const LRESULT custom = HandleNcHitTest(hwnd, lparam);
      if (custom != HTCLIENT) {
        return custom;
      }
      // Caption buttons stay HTCLIENT even if DefWindowProc reports HTCAPTION.
      return HTCLIENT;
    }
    case WM_NCLBUTTONDOWN:
      if (wparam == HTCAPTION) {
        POINT cursor{};
        GetCursorPos(&cursor);
        ScreenToClient(hwnd, &cursor);
        PostMessage(hwnd, WM_MOUSEMOVE, 0, MAKELPARAM(cursor.x, cursor.y));
      }
      return DefWindowProc(hwnd, message, wparam, lparam);
    case WM_NCLBUTTONDBLCLK:
      return DefWindowProc(hwnd, message, wparam, lparam);
    case WM_GETMINMAXINFO:
      HandleMinMaxInfo(hwnd, lparam);
      return 0;
    case WM_DESTROY:
      window_handle_ = nullptr;
      Destroy();
      if (quit_on_close_) {
        PostQuitMessage(0);
      }
      return 0;

    case WM_DPICHANGED: {
      auto newRectSize = reinterpret_cast<RECT*>(lparam);
      LONG newWidth = newRectSize->right - newRectSize->left;
      LONG newHeight = newRectSize->bottom - newRectSize->top;

      SetWindowPos(hwnd, nullptr, newRectSize->left, newRectSize->top, newWidth,
                   newHeight, SWP_NOZORDER | SWP_NOACTIVATE | SWP_FRAMECHANGED);
      ApplyCustomFrame();
      return 0;
    }
    case WM_SIZE: {
      RECT rect = GetClientArea();
      if (child_content_ != nullptr) {
        MoveWindow(child_content_, rect.left, rect.top, rect.right - rect.left,
                   rect.bottom - rect.top, TRUE);
      }
      return 0;
    }

    case WM_DWMCOLORIZATIONCOLORCHANGED:
      UpdateTheme(hwnd);
      return 0;
  }

  return DefWindowProc(window_handle_, message, wparam, lparam);
}

void Win32Window::Destroy() {
  OnDestroy();

  if (window_handle_) {
    DestroyWindow(window_handle_);
    window_handle_ = nullptr;
  }
  if (g_active_window_count == 0) {
    WindowClassRegistrar::GetInstance()->UnregisterWindowClass();
  }
}

Win32Window* Win32Window::GetThisFromHandle(HWND const window) noexcept {
  return reinterpret_cast<Win32Window*>(
      GetWindowLongPtr(window, GWLP_USERDATA));
}

void Win32Window::SetChildContent(HWND content) {
  child_content_ = content;
  SetParent(content, window_handle_);
  RECT frame = GetClientArea();

  MoveWindow(content, frame.left, frame.top, frame.right - frame.left,
             frame.bottom - frame.top, true);

  SetFocus(child_content_);
}

RECT Win32Window::GetClientArea() {
  RECT frame;
  GetClientRect(window_handle_, &frame);
  return frame;
}

HWND Win32Window::GetHandle() { return window_handle_; }

void Win32Window::SetQuitOnClose(bool quit_on_close) {
  quit_on_close_ = quit_on_close;
}

bool Win32Window::OnCreate() { return true; }

void Win32Window::OnDestroy() {}

void Win32Window::UpdateTheme(HWND const window) {
  DWORD light_mode;
  DWORD light_mode_size = sizeof(light_mode);
  LSTATUS result = RegGetValue(HKEY_CURRENT_USER, kGetPreferredBrightnessRegKey,
                               kGetPreferredBrightnessRegValue, RRF_RT_REG_DWORD,
                               nullptr, &light_mode, &light_mode_size);

  if (result == ERROR_SUCCESS) {
    BOOL enable_dark_mode = light_mode == 0;
    DwmSetWindowAttribute(window, DWMWA_USE_IMMERSIVE_DARK_MODE,
                          &enable_dark_mode, sizeof(enable_dark_mode));
  }
}

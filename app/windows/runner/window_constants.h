#ifndef RUNNER_WINDOW_CONSTANTS_H_
#define RUNNER_WINDOW_CONSTANTS_H_

#include <windows.h>

constexpr wchar_t kNagaWindowClass[] = L"NagaNetworkWindow";
constexpr wchar_t kNagaMutexName[] = L"Local\\NagaNetworkUI";
constexpr wchar_t kNagaReadyEventName[] = L"Local\\NagaNetworkWindowReady";
constexpr wchar_t kNagaAppUserModelId[] = L"eu.nagavpn.naga_network";

constexpr UINT kNagaTrayCallback = WM_APP + 1;
constexpr UINT kNagaShowFromOtherProcess = WM_APP + 2;
constexpr ULONG_PTR kNagaCopyInstallConfig = 0x4E414741;

// Logical pixels; must match Dart nagaTitleBarHeight / nagaCaptionButtonSpan
// and NagaBreakpoints.minWindowWidth / minWindowHeight.
constexpr int kNagaTitleBarHeight = 40;
constexpr int kNagaCaptionButtonSpan = 132;
constexpr int kNagaMinWindowWidth = 390;
constexpr int kNagaMinWindowHeight = 560;

#endif  // RUNNER_WINDOW_CONSTANTS_H_

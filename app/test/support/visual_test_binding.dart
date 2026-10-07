import 'package:flutter_test/flutter_test.dart';

/// Снимки с настоящим размытием теней, как в приложении.
/// Стандартная тестовая привязка заменяет тени плоскими заливками.
class NagaVisualTestBinding extends AutomatedTestWidgetsFlutterBinding {
  @override
  bool get disableShadows => false;
}

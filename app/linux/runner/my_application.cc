#include "my_application.h"

#include <libayatana-appindicator/app-indicator.h>
#include <flutter_linux/flutter_linux.h>
#include <gio/gio.h>
#include <limits.h>
#include <unistd.h>

#ifdef GDK_WINDOWING_X11
#include <gdk/gdkx.h>
#endif

#include "flutter/generated_plugin_registrant.h"

struct _MyApplication {
  GtkApplication parent_instance;
  char** dart_entrypoint_arguments;
  GtkWindow* window;
  FlView* view;
  FlMethodChannel* tray_channel;
  FlMethodChannel* update_channel;
  AppIndicator* indicator;
  GtkWidget* tray_status_item;
  GtkWidget* tray_profile_item;
  GtkWidget* tray_toggle_item;
  gboolean quitting;
  gchar* pending_install_uri;
};

G_DEFINE_TYPE(MyApplication, my_application, GTK_TYPE_APPLICATION)

static void invoke_install_config(MyApplication* self, const gchar* uri) {
  if (uri == nullptr || uri[0] == '\0') {
    return;
  }
  if (self->tray_channel == nullptr) {
    g_free(self->pending_install_uri);
    self->pending_install_uri = g_strdup(uri);
    return;
  }
  g_autoptr(FlValue) args = fl_value_new_string(uri);
  fl_method_channel_invoke_method(self->tray_channel, "installConfig", args,
                                  nullptr, nullptr, nullptr);
}

static void flush_pending_install_config(MyApplication* self) {
  if (self->pending_install_uri == nullptr) {
    return;
  }
  gchar* uri = self->pending_install_uri;
  self->pending_install_uri = nullptr;
  invoke_install_config(self, uri);
  g_free(uri);
}

static void tray_invoke(MyApplication* self, const gchar* method) {
  if (self->tray_channel == nullptr) return;
  g_autoptr(FlValue) args = fl_value_new_null();
  fl_method_channel_invoke_method(self->tray_channel, method, args, nullptr,
                                  nullptr, nullptr);
}

static void tray_show_window(MyApplication* self) {
  if (self->window == nullptr) return;
  gtk_widget_show(GTK_WIDGET(self->window));
  gtk_window_present(self->window);
}

static void tray_open_cb(GtkMenuItem* item, gpointer user_data) {
  MyApplication* self = MY_APPLICATION(user_data);
  tray_show_window(self);
}

static void tray_toggle_cb(GtkMenuItem* item, gpointer user_data) {
  tray_invoke(MY_APPLICATION(user_data), "toggleConnection");
}

static void tray_server_cb(GtkMenuItem* item, gpointer user_data) {
  MyApplication* self = MY_APPLICATION(user_data);
  tray_show_window(self);
  tray_invoke(self, "openServerPicker");
}

static void tray_quit_cb(GtkMenuItem* item, gpointer user_data) {
  MyApplication* self = MY_APPLICATION(user_data);
  tray_invoke(self, "quitApplication");
}

static gboolean window_delete_cb(GtkWidget* widget,
                                 GdkEvent* event,
                                 gpointer user_data) {
  MyApplication* self = MY_APPLICATION(user_data);
  if (self->quitting) return FALSE;
  gtk_widget_hide(widget);
  return TRUE;
}

static const gchar* tray_status_text(const gchar* status) {
  if (g_strcmp0(status, "connected") == 0) return "● Соединение защищено";
  if (g_strcmp0(status, "connecting") == 0) return "◌ Подключение…";
  if (g_strcmp0(status, "error") == 0) return "! Ошибка подключения";
  return "○ VPN отключён";
}

static void tray_update(MyApplication* self,
                        const gchar* status,
                        const gchar* profile,
                        const gchar* server) {
  if (self->tray_status_item == nullptr) return;
  gtk_menu_item_set_label(GTK_MENU_ITEM(self->tray_status_item),
                          tray_status_text(status));
  gtk_menu_item_set_label(GTK_MENU_ITEM(self->tray_profile_item),
                          profile != nullptr && profile[0] != '\0'
                              ? profile
                              : "Профиль не выбран");
  gtk_menu_item_set_label(GTK_MENU_ITEM(self->tray_toggle_item),
                          g_strcmp0(status, "connected") == 0
                              ? "Отключиться"
                              : "Подключиться");
  gchar* tooltip = g_strdup_printf("Naga Network — %s",
                                   server != nullptr && server[0] != '\0'
                                       ? server
                                       : tray_status_text(status));
  app_indicator_set_title(self->indicator, tooltip);
  g_free(tooltip);
}

static void tray_method_call_cb(FlMethodChannel* channel,
                                FlMethodCall* method_call,
                                gpointer user_data) {
  MyApplication* self = MY_APPLICATION(user_data);
  const gchar* method = fl_method_call_get_name(method_call);
  g_autoptr(FlMethodResponse) response = nullptr;

  if (g_strcmp0(method, "updateStatus") == 0) {
    FlValue* args = fl_method_call_get_args(method_call);
    const gchar* status = "disconnected";
    const gchar* profile = nullptr;
    const gchar* server = nullptr;
    if (args != nullptr && fl_value_get_type(args) == FL_VALUE_TYPE_MAP) {
      FlValue* value = fl_value_lookup_string(args, "status");
      if (value != nullptr && fl_value_get_type(value) == FL_VALUE_TYPE_STRING)
        status = fl_value_get_string(value);
      value = fl_value_lookup_string(args, "profile");
      if (value != nullptr && fl_value_get_type(value) == FL_VALUE_TYPE_STRING)
        profile = fl_value_get_string(value);
      value = fl_value_lookup_string(args, "server");
      if (value != nullptr && fl_value_get_type(value) == FL_VALUE_TYPE_STRING)
        server = fl_value_get_string(value);
    }
    tray_update(self, status, profile, server);
    response = FL_METHOD_RESPONSE(fl_method_success_response_new(nullptr));
  } else if (g_strcmp0(method, "terminate") == 0) {
    self->quitting = TRUE;
    response = FL_METHOD_RESPONSE(fl_method_success_response_new(nullptr));
    g_application_quit(G_APPLICATION(self));
  } else {
    response = FL_METHOD_RESPONSE(fl_method_not_implemented_response_new());
  }

  g_autoptr(GError) error = nullptr;
  if (!fl_method_call_respond(method_call, response, &error) && error != nullptr)
    g_warning("Failed to respond to tray method: %s", error->message);
}

static void setup_tray(MyApplication* self) {
  GtkWidget* menu = gtk_menu_new();
  GtkWidget* title = gtk_menu_item_new_with_label("Naga Network");
  GtkWidget* status = gtk_menu_item_new_with_label("○ VPN отключён");
  GtkWidget* profile = gtk_menu_item_new_with_label("Профиль не выбран");
  GtkWidget* separator = gtk_separator_menu_item_new();
  GtkWidget* toggle = gtk_menu_item_new_with_label("Подключиться");
  GtkWidget* server = gtk_menu_item_new_with_label("Сменить сервер");
  GtkWidget* open = gtk_menu_item_new_with_label("Открыть Naga Network");
  GtkWidget* quit_separator = gtk_separator_menu_item_new();
  GtkWidget* quit = gtk_menu_item_new_with_label("Выход");

  gtk_widget_set_sensitive(title, FALSE);
  gtk_widget_set_sensitive(status, FALSE);
  gtk_widget_set_sensitive(profile, FALSE);
  gtk_menu_shell_append(GTK_MENU_SHELL(menu), title);
  gtk_menu_shell_append(GTK_MENU_SHELL(menu), status);
  gtk_menu_shell_append(GTK_MENU_SHELL(menu), profile);
  gtk_menu_shell_append(GTK_MENU_SHELL(menu), separator);
  gtk_menu_shell_append(GTK_MENU_SHELL(menu), toggle);
  gtk_menu_shell_append(GTK_MENU_SHELL(menu), server);
  gtk_menu_shell_append(GTK_MENU_SHELL(menu), open);
  gtk_menu_shell_append(GTK_MENU_SHELL(menu), quit_separator);
  gtk_menu_shell_append(GTK_MENU_SHELL(menu), quit);
  gtk_widget_show_all(menu);

  self->tray_status_item = status;
  self->tray_profile_item = profile;
  self->tray_toggle_item = toggle;
  g_signal_connect(toggle, "activate", G_CALLBACK(tray_toggle_cb), self);
  g_signal_connect(server, "activate", G_CALLBACK(tray_server_cb), self);
  g_signal_connect(open, "activate", G_CALLBACK(tray_open_cb), self);
  g_signal_connect(quit, "activate", G_CALLBACK(tray_quit_cb), self);

#pragma GCC diagnostic push
#pragma GCC diagnostic ignored "-Wdeprecated-declarations"
  self->indicator = app_indicator_new(
      "naga-network", "network-vpn", APP_INDICATOR_CATEGORY_APPLICATION_STATUS);
#pragma GCC diagnostic pop
  gchar executable[PATH_MAX];
  ssize_t length = readlink("/proc/self/exe", executable, sizeof(executable) - 1);
  if (length > 0) {
    executable[length] = '\0';
    gchar* binary_dir = g_path_get_dirname(executable);
    gchar* icon_dir = g_build_filename(binary_dir, "data", "flutter_assets",
                                       "assets", "brand", nullptr);
    app_indicator_set_icon_theme_path(self->indicator, icon_dir);
    app_indicator_set_icon_full(self->indicator, "naga-mark-red", "Naga Network");
    g_free(icon_dir);
    g_free(binary_dir);
  }
  app_indicator_set_status(self->indicator, APP_INDICATOR_STATUS_ACTIVE);
  app_indicator_set_menu(self->indicator, GTK_MENU(menu));
}

static gchar* map_string_arg(FlValue* args, const gchar* key) {
  if (args == nullptr || fl_value_get_type(args) != FL_VALUE_TYPE_MAP) {
    return nullptr;
  }
  FlValue* value = fl_value_lookup_string(args, key);
  if (value == nullptr || fl_value_get_type(value) != FL_VALUE_TYPE_STRING) {
    return nullptr;
  }
  return g_strdup(fl_value_get_string(value));
}

static void update_method_call_cb(FlMethodChannel* channel,
                                  FlMethodCall* method_call,
                                  gpointer user_data) {
  (void)channel;
  MyApplication* self = MY_APPLICATION(user_data);
  const gchar* method = fl_method_call_get_name(method_call);
  g_autoptr(FlMethodResponse) response = nullptr;
  FlValue* args = fl_method_call_get_args(method_call);

  if (g_strcmp0(method, "restartSystemd") == 0) {
    g_autofree gchar* unit = map_string_arg(args, "unit");
    if (unit == nullptr || unit[0] == '\0') {
      response = FL_METHOD_RESPONSE(fl_method_error_response_new(
          "unit", "systemd unit is required", nullptr));
    } else {
      gchar* argv[] = {(gchar*)"pkexec", (gchar*)"systemctl", (gchar*)"restart",
                       unit, nullptr};
      gint status = 0;
      g_autoptr(GError) spawn_error = nullptr;
      if (!g_spawn_sync(nullptr, argv, nullptr, G_SPAWN_SEARCH_PATH, nullptr,
                        nullptr, nullptr, nullptr, &status, &spawn_error) ||
          status != 0) {
        response = FL_METHOD_RESPONSE(fl_method_error_response_new(
            "pkexec", "pkexec systemctl restart failed", nullptr));
      } else {
        response = FL_METHOD_RESPONSE(fl_method_success_response_new(nullptr));
      }
    }
  } else if (g_strcmp0(method, "launchAndQuit") == 0) {
    g_autofree gchar* path = map_string_arg(args, "path");
    if (path == nullptr || path[0] == '\0') {
      response = FL_METHOD_RESPONSE(fl_method_error_response_new(
          "path", "AppImage path is required", nullptr));
    } else {
      gchar* argv[] = {path, nullptr};
      g_autoptr(GError) spawn_error = nullptr;
      if (!g_spawn_async(nullptr, argv, nullptr,
                         (GSpawnFlags)(G_SPAWN_DO_NOT_REAP_CHILD), nullptr,
                         nullptr, nullptr, &spawn_error)) {
        response = FL_METHOD_RESPONSE(fl_method_error_response_new(
            "spawn", "failed to launch AppImage", nullptr));
      } else {
        self->quitting = TRUE;
        response = FL_METHOD_RESPONSE(fl_method_success_response_new(nullptr));
        g_application_quit(G_APPLICATION(self));
      }
    }
  } else {
    response = FL_METHOD_RESPONSE(fl_method_not_implemented_response_new());
  }

  g_autoptr(GError) error = nullptr;
  if (!fl_method_call_respond(method_call, response, &error) && error != nullptr)
    g_warning("Failed to respond to update method: %s", error->message);
}

static void create_update_channel(MyApplication* self) {
  FlEngine* engine = fl_view_get_engine(self->view);
  FlBinaryMessenger* messenger = fl_engine_get_binary_messenger(engine);
  g_autoptr(FlStandardMethodCodec) codec = fl_standard_method_codec_new();
  self->update_channel = fl_method_channel_new(
      messenger, "eu.nagavpn.naga_network/update", FL_METHOD_CODEC(codec));
  fl_method_channel_set_method_call_handler(
      self->update_channel, update_method_call_cb, self, nullptr);
}

static void create_tray_channel(MyApplication* self) {
  FlEngine* engine = fl_view_get_engine(self->view);
  FlBinaryMessenger* messenger = fl_engine_get_binary_messenger(engine);
  g_autoptr(FlStandardMethodCodec) codec = fl_standard_method_codec_new();
  self->tray_channel = fl_method_channel_new(
      messenger, "eu.nagavpn.naga_network/tray", FL_METHOD_CODEC(codec));
  fl_method_channel_set_method_call_handler(self->tray_channel,
                                            tray_method_call_cb, self, nullptr);
  flush_pending_install_config(self);
}

// Called when first Flutter frame received.
static void first_frame_cb(MyApplication* self, FlView* view) {
  gtk_widget_show(gtk_widget_get_toplevel(GTK_WIDGET(view)));
}

// Implements GApplication::activate.
static void my_application_activate(GApplication* application) {
  MyApplication* self = MY_APPLICATION(application);
  if (self->window != nullptr) {
    gtk_widget_show(GTK_WIDGET(self->window));
    gtk_window_present(self->window);
    return;
  }

  GtkWindow* window =
      GTK_WINDOW(gtk_application_window_new(GTK_APPLICATION(application)));
  self->window = window;

  // Use a header bar when running in GNOME as this is the common style used
  // by applications and is the setup most users will be using (e.g. Ubuntu
  // desktop).
  // If running on X and not using GNOME then just use a traditional title bar
  // in case the window manager does more exotic layout, e.g. tiling.
  // If running on Wayland assume the header bar will work (may need changing
  // if future cases occur).
  gboolean use_header_bar = TRUE;
#ifdef GDK_WINDOWING_X11
  GdkScreen* screen = gtk_window_get_screen(window);
  if (GDK_IS_X11_SCREEN(screen)) {
    const gchar* wm_name = gdk_x11_screen_get_window_manager_name(screen);
    if (g_strcmp0(wm_name, "GNOME Shell") != 0) {
      use_header_bar = FALSE;
    }
  }
#endif
  if (use_header_bar) {
    GtkHeaderBar* header_bar = GTK_HEADER_BAR(gtk_header_bar_new());
    gtk_widget_show(GTK_WIDGET(header_bar));
    gtk_header_bar_set_title(header_bar, "Naga Network");
    gtk_header_bar_set_show_close_button(header_bar, TRUE);
    gtk_window_set_titlebar(window, GTK_WIDGET(header_bar));
  } else {
    gtk_window_set_title(window, "Naga Network");
  }

  gtk_window_set_default_size(window, 1280, 720);

  g_autoptr(FlDartProject) project = fl_dart_project_new();
  fl_dart_project_set_dart_entrypoint_arguments(
      project, self->dart_entrypoint_arguments);

  FlView* view = fl_view_new(project);
  self->view = view;
  GdkRGBA background_color;
  // Background defaults to black, override it here if necessary, e.g. #00000000
  // for transparent.
  gdk_rgba_parse(&background_color, "#000000");
  fl_view_set_background_color(view, &background_color);
  gtk_widget_show(GTK_WIDGET(view));
  gtk_container_add(GTK_CONTAINER(window), GTK_WIDGET(view));

  // Show the window when Flutter renders.
  // Requires the view to be realized so we can start rendering.
  g_signal_connect_swapped(view, "first-frame", G_CALLBACK(first_frame_cb),
                           self);
  gtk_widget_realize(GTK_WIDGET(view));

  fl_register_plugins(FL_PLUGIN_REGISTRY(view));
  create_tray_channel(self);
  create_update_channel(self);
  setup_tray(self);
  g_signal_connect(window, "delete-event", G_CALLBACK(window_delete_cb), self);

  gtk_widget_grab_focus(GTK_WIDGET(view));
}

// Implements GApplication::local_command_line.
static gboolean my_application_local_command_line(GApplication* application,
                                                  gchar*** arguments,
                                                  int* exit_status) {
  MyApplication* self = MY_APPLICATION(application);
  // Strip out the first argument as it is the binary name.
  self->dart_entrypoint_arguments = g_strdupv(*arguments + 1);

  g_autoptr(GError) error = nullptr;
  if (!g_application_register(application, nullptr, &error)) {
    g_warning("Failed to register: %s", error->message);
    *exit_status = 1;
    return TRUE;
  }

  // Unique apps: a second process is remote. Forward install URIs to the
  // primary instead of dropping argv on activate-only.
  if (g_application_get_is_remote(application)) {
    gint argc = 0;
    for (gchar** cursor = self->dart_entrypoint_arguments;
         cursor != nullptr && *cursor != nullptr; cursor++) {
      argc++;
    }
    if (argc > 0) {
      GFile** files = g_new0(GFile*, argc);
      gint n_files = 0;
      for (gint i = 0; i < argc; i++) {
        const gchar* arg = self->dart_entrypoint_arguments[i];
        if (g_str_has_prefix(arg, "nagavpn:") || g_str_has_prefix(arg, "https:")) {
          files[n_files++] = g_file_new_for_uri(arg);
        }
      }
      if (n_files > 0) {
        g_application_open(application, files, n_files, "");
      }
      for (gint i = 0; i < n_files; i++) {
        g_object_unref(files[i]);
      }
      g_free(files);
    }
    g_application_activate(application);
    *exit_status = 0;
    return TRUE;
  }

  g_application_activate(application);
  *exit_status = 0;
  return TRUE;
}

static void my_application_open(GApplication* application,
                                GFile** files,
                                gint n_files,
                                const gchar* hint) {
  (void)hint;
  MyApplication* self = MY_APPLICATION(application);
  g_application_activate(application);
  tray_show_window(self);
  for (gint i = 0; i < n_files; i++) {
    g_autofree gchar* uri = g_file_get_uri(files[i]);
    invoke_install_config(self, uri);
  }
}

// Implements GApplication::startup.
static void my_application_startup(GApplication* application) {
  // MyApplication* self = MY_APPLICATION(object);

  // Perform any actions required at application startup.

  G_APPLICATION_CLASS(my_application_parent_class)->startup(application);
}

// Implements GApplication::shutdown.
static void my_application_shutdown(GApplication* application) {
  // MyApplication* self = MY_APPLICATION(object);

  // Perform any actions required at application shutdown.

  G_APPLICATION_CLASS(my_application_parent_class)->shutdown(application);
}

// Implements GObject::dispose.
static void my_application_dispose(GObject* object) {
  MyApplication* self = MY_APPLICATION(object);
  if (self->indicator != nullptr) {
    app_indicator_set_status(self->indicator, APP_INDICATOR_STATUS_PASSIVE);
    g_clear_object(&self->indicator);
  }
  g_clear_object(&self->tray_channel);
  g_clear_object(&self->update_channel);
  self->window = nullptr;
  self->view = nullptr;
  g_clear_pointer(&self->dart_entrypoint_arguments, g_strfreev);
  g_clear_pointer(&self->pending_install_uri, g_free);
  G_OBJECT_CLASS(my_application_parent_class)->dispose(object);
}

static void my_application_class_init(MyApplicationClass* klass) {
  G_APPLICATION_CLASS(klass)->activate = my_application_activate;
  G_APPLICATION_CLASS(klass)->local_command_line =
      my_application_local_command_line;
  G_APPLICATION_CLASS(klass)->open = my_application_open;
  G_APPLICATION_CLASS(klass)->startup = my_application_startup;
  G_APPLICATION_CLASS(klass)->shutdown = my_application_shutdown;
  G_OBJECT_CLASS(klass)->dispose = my_application_dispose;
}

static void my_application_init(MyApplication* self) {}

MyApplication* my_application_new() {
  // Set the program name to the application ID, which helps various systems
  // like GTK and desktop environments map this running application to its
  // corresponding .desktop file. This ensures better integration by allowing
  // the application to be recognized beyond its binary name.
  g_set_prgname(APPLICATION_ID);

  return MY_APPLICATION(g_object_new(my_application_get_type(),
                                     "application-id", APPLICATION_ID, "flags",
                                     G_APPLICATION_DEFAULT_FLAGS |
                                         G_APPLICATION_HANDLES_OPEN,
                                     nullptr));
}

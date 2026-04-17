use tauri::Manager;
use tauri_plugin_shell::ShellExt;
use tauri_plugin_updater::UpdaterExt;

#[tauri::command]
async fn check_update(app: tauri::AppHandle) -> Result<bool, String> {
  let updater = app
    .updater()
    .map_err(|e| format!("Failed to get updater: {e}"))?;

  match updater.check().await {
    Ok(Some(update)) => {
      update
        .download_and_install(|_, _| {}, || {})
        .await
        .map_err(|e| format!("Failed to install update: {e}"))?;
      Ok(true)
    }
    Ok(None) => Ok(false),
    Err(e) => Err(format!("Update check failed: {e}")),
  }
}

#[cfg_attr(mobile, tauri::mobile_entry_point)]
pub fn run() {
  tauri::Builder::default()
    .plugin(tauri_plugin_shell::init())
    .plugin(tauri_plugin_updater::Builder::new().build())
    .plugin(tauri_plugin_process::init())
    .plugin(tauri_plugin_dialog::init())
    .setup(|app| {
      // Apply overlay title bar on macOS so content extends under the traffic lights
      #[cfg(target_os = "macos")]
      {
        let window = app.get_webview_window("main").unwrap();
        use tauri::TitleBarStyle;
        window.set_title_bar_style(TitleBarStyle::Overlay).unwrap();
      }

      // Spawn the sidecar backend.
      // POLVO_ROOT is read from the environment (set by the Makefile/launcher)
      // and forwarded to the sidecar so it finds polvo.yaml in the correct
      // project directory instead of defaulting to the Tauri resource dir.
      // Priority: CLI arg > POLVO_ROOT env > current dir
      // e.g. `polvo /path/to/project` or `polvo .`
      let args: Vec<String> = std::env::args().collect();
      let reset = args.iter().any(|a| a == "--reset" || a == "--clean");

      // --reset / --clean: wipe WebKit localStorage so the app starts fresh.
      if reset {
        // On macOS: ~/Library/WebKit/<identifier>/WebsiteData
        #[cfg(target_os = "macos")]
        if let Ok(home) = app.path().home_dir() {
          let webkit_data = home
            .join("Library/WebKit")
            .join(app.config().identifier.clone())
            .join("WebsiteData");
          if webkit_data.exists() {
            let _ = std::fs::remove_dir_all(&webkit_data);
            log::info!("--reset: cleared WebKit storage at {:?}", webkit_data);
          }
        }
      }

      let polvo_root = args.iter()
        .skip(1)
        .find(|a| !a.starts_with("--"))
        .and_then(|arg| {
          let path = std::path::Path::new(arg);
          if path.is_dir() {
            path.canonicalize().ok().map(|p| p.to_string_lossy().to_string())
          } else {
            None
          }
        })
        .or_else(|| std::env::var("POLVO_ROOT").ok())
        .unwrap_or_else(|| {
          std::env::current_dir()
            .unwrap_or_default()
            .to_string_lossy()
            .to_string()
        });

      match app.shell().sidecar("polvo") {
        Ok(sidecar_command) => {
          match sidecar_command.env("POLVO_ROOT", &polvo_root).env("POLVO_SIDECAR", "1").spawn() {
            Ok(_) => {}
            Err(e) => {
              log::warn!("polvo sidecar could not be spawned (dev mode?): {e}");
            }
          }
        }
        Err(e) => {
          log::warn!("polvo sidecar not found (dev mode?): {e}");
        }
      }

      if cfg!(debug_assertions) {
        app.handle().plugin(
          tauri_plugin_log::Builder::default()
            .level(log::LevelFilter::Info)
            .build(),
        )?;
      }
      Ok(())
    })
    .invoke_handler(tauri::generate_handler![check_update])
    .run(tauri::generate_context!())
    .expect("error while running tauri application");
}

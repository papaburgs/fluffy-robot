package app

// // Backup saves the app data to disk so we can pick it up in the case of a restart
// func (a *App) Backup() {
// 	jsonData, err := json.Marshal(a)
// 	if err != nil {
// 		slog.Error("failed to marshal data to JSON", "error", err)
// 		return
// 	}
//
// 	if err := os.WriteFile(genBackupPath(backupLocation), jsonData, 0644); err != nil {
// 		slog.Error("failed to write data to file", "path", genBackupPath(backupLocation), "error", err)
// 		return
// 	}
//
// 	slog.Info("Successfully wrote data")
// 	return
// }
//
// // RestoreData reads the JSON content from the specified file path and deserializes
// // it into the app
// // hopefully this means the app can survive restarts/sleeps
// func (a *App) Restore() {
// 	jsonData, err := os.ReadFile(genBackupPath(backupLocation))
// 	if err != nil {
// 		slog.Error("failed to read file", "path", genBackupPath(backupLocation), "error", err)
// 		return
// 	}
//
// 	if err := json.Unmarshal(jsonData, a); err != nil {
// 		slog.Error("failed to unmarshal JSON data", "error", err)
// 		return
// 	}
//
// 	slog.Info("Successfully restored data", "path", genBackupPath(backupLocation))
// }
//
// func genBackupPath(base string) string {
// 	return fmt.Sprintf("%s/backup.json", base)
// }

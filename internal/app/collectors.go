package app

// func (a *App) updateServerStatus(data ServerStatus) {
// 	mapLock.Lock()
// 	defer mapLock.Unlock()
// 	if a.Reset != data.ResetDate {
// 		slog.Info("new reset date")
// 		a.Reset = data.ResetDate
// 		a.LastReset = a.Current
// 		a.Current = make(map[string][]AgentRecord)
// 	}
// 	if data.Stats.Accounts == nil {
// 		a.Accounts = 0
// 	} else {
// 		a.Accounts = *data.Stats.Accounts
// 	}
// 	a.Agents = data.Stats.Agents
// 	a.Ships = data.Stats.Ships
// }

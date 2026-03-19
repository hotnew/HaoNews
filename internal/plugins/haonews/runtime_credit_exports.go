package newsplugin

import "hao.news/internal/aip2p"

func (a *App) CreditBalance(author string) (aip2p.CreditBalance, error) {
	store, err := aip2p.OpenCreditStore(a.storeRoot)
	if err != nil {
		return aip2p.CreditBalance{}, err
	}
	return store.GetBalance(author), nil
}

func (a *App) CreditBalances() ([]aip2p.CreditBalance, error) {
	store, err := aip2p.OpenCreditStore(a.storeRoot)
	if err != nil {
		return nil, err
	}
	return store.GetAllBalances()
}

func (a *App) CreditProofsByDate(date string) ([]aip2p.OnlineProof, error) {
	store, err := aip2p.OpenCreditStore(a.storeRoot)
	if err != nil {
		return nil, err
	}
	return store.GetProofsByDate(date)
}

func (a *App) CreditProofsByAuthor(author, start, end string) ([]aip2p.OnlineProof, error) {
	store, err := aip2p.OpenCreditStore(a.storeRoot)
	if err != nil {
		return nil, err
	}
	return store.GetProofsByAuthor(author, start, end)
}

func (a *App) CreditIntegrityIssues() ([]string, error) {
	store, err := aip2p.OpenCreditStore(a.storeRoot)
	if err != nil {
		return nil, err
	}
	return store.ValidateBalanceIntegrity(), nil
}

func (a *App) CreditDailyStats(limit int) ([]aip2p.CreditDailyStat, error) {
	store, err := aip2p.OpenCreditStore(a.storeRoot)
	if err != nil {
		return nil, err
	}
	return store.GetDailyStats(limit)
}

func (a *App) CreditWitnessRoleStats() ([]aip2p.CreditWitnessRoleStat, error) {
	store, err := aip2p.OpenCreditStore(a.storeRoot)
	if err != nil {
		return nil, err
	}
	return store.GetWitnessRoleStats()
}

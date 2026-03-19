package newsplugin

import "hao.news/internal/haonews"

func (a *App) CreditBalance(author string) (haonews.CreditBalance, error) {
	store, err := haonews.OpenCreditStore(a.storeRoot)
	if err != nil {
		return haonews.CreditBalance{}, err
	}
	return store.GetBalance(author), nil
}

func (a *App) CreditBalances() ([]haonews.CreditBalance, error) {
	store, err := haonews.OpenCreditStore(a.storeRoot)
	if err != nil {
		return nil, err
	}
	return store.GetAllBalances()
}

func (a *App) CreditProofsByDate(date string) ([]haonews.OnlineProof, error) {
	store, err := haonews.OpenCreditStore(a.storeRoot)
	if err != nil {
		return nil, err
	}
	return store.GetProofsByDate(date)
}

func (a *App) CreditProofsByAuthor(author, start, end string) ([]haonews.OnlineProof, error) {
	store, err := haonews.OpenCreditStore(a.storeRoot)
	if err != nil {
		return nil, err
	}
	return store.GetProofsByAuthor(author, start, end)
}

func (a *App) CreditIntegrityIssues() ([]string, error) {
	store, err := haonews.OpenCreditStore(a.storeRoot)
	if err != nil {
		return nil, err
	}
	return store.ValidateBalanceIntegrity(), nil
}

func (a *App) CreditDailyStats(limit int) ([]haonews.CreditDailyStat, error) {
	store, err := haonews.OpenCreditStore(a.storeRoot)
	if err != nil {
		return nil, err
	}
	return store.GetDailyStats(limit)
}

func (a *App) CreditWitnessRoleStats() ([]haonews.CreditWitnessRoleStat, error) {
	store, err := haonews.OpenCreditStore(a.storeRoot)
	if err != nil {
		return nil, err
	}
	return store.GetWitnessRoleStats()
}

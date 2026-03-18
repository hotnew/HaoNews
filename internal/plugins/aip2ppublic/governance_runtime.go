package newsplugin

func (a *App) governanceIndex(index Index) (Index, error) {
	delegations, err := LoadDelegationStore(delegationDirForWriterPolicy(a.writerPath), revocationDirForWriterPolicy(a.writerPath))
	if err != nil {
		return Index{}, err
	}
	if a.loadWriter == nil {
		return applyDelegationMetadata(index, a.project, delegations), nil
	}
	policy, err := a.loadWriter(a.writerPath)
	if err != nil {
		return Index{}, err
	}
	return ApplyWriterPolicyWithDelegations(index, a.project, policy, delegations), nil
}

func (a *App) governanceSummary() []SummaryStat {
	policy, err := a.loadWriter(a.writerPath)
	return writerPolicySummary(policy, err)
}

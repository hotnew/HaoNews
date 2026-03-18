package newsplugin

func FullAppOptions() AppOptions {
	return AppOptions{
		ContentRoutes:      true,
		ContentAPIRoutes:   true,
		ArchiveRoutes:      true,
		HistoryAPIRoutes:   true,
		NetworkRoutes:      true,
		NetworkAPIRoutes:   true,
		WriterPolicyRoutes: true,
	}
}

func ContentOnlyAppOptions() AppOptions {
	return AppOptions{
		ContentRoutes:    true,
		ContentAPIRoutes: true,
	}
}

func ArchiveOnlyAppOptions() AppOptions {
	return AppOptions{
		ArchiveRoutes:    true,
		HistoryAPIRoutes: true,
	}
}

func GovernanceOnlyAppOptions() AppOptions {
	return AppOptions{
		WriterPolicyRoutes: true,
	}
}

func OpsOnlyAppOptions() AppOptions {
	return AppOptions{
		NetworkRoutes:    true,
		NetworkAPIRoutes: true,
	}
}

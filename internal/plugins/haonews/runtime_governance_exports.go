package newsplugin

import (
	"path/filepath"
	"strconv"
	"strings"
)

func (a *App) WriterPolicyPath() string {
	return a.writerPath
}

func (a *App) RulesPath() string {
	return a.rulesPath
}

func (a *App) GovernanceSummary() []SummaryStat {
	return a.governanceSummary()
}

func writerPolicySummary(policy WriterPolicy, err error) []SummaryStat {
	if err != nil {
		return []SummaryStat{
			{Label: "Effective policy", Value: "load error"},
			{Label: "Reason", Value: err.Error()},
		}
	}
	return []SummaryStat{
		{Label: "Sync mode", Value: string(policy.SyncMode)},
		{Label: "Trusted authorities", Value: strconv.Itoa(len(policy.TrustedAuthorities))},
		{Label: "Shared registries", Value: strconv.Itoa(len(policy.SharedRegistries))},
		{Label: "Writer caps", Value: strconv.Itoa(len(policy.AgentCapabilities) + len(policy.PublicKeyCapabilities))},
		{Label: "Relay trust rules", Value: strconv.Itoa(len(policy.RelayPeerTrust) + len(policy.RelayHostTrust))},
	}
}

func DefaultWriterPolicy() WriterPolicy {
	return defaultWriterPolicy()
}

func WriterWhitelistPath(writerPolicyPath string) string {
	return filepath.Join(filepath.Dir(writerPolicyPath), writerWhitelistINFName)
}

func WriterBlacklistPath(writerPolicyPath string) string {
	return filepath.Join(filepath.Dir(writerPolicyPath), writerBlacklistINFName)
}

func originSummary(origin *MessageOrigin) (author, agentID, keyType, publicKey string, signed bool) {
	if origin == nil {
		return "", "", "", "", false
	}
	return strings.TrimSpace(origin.Author),
		strings.TrimSpace(origin.AgentID),
		strings.TrimSpace(origin.KeyType),
		strings.TrimSpace(origin.PublicKey),
		strings.TrimSpace(origin.Signature) != ""
}

func delegationSummary(info *DelegationInfo) (delegated bool, parentAgentID, parentKeyType, parentPublicKey string) {
	if info == nil || !info.Delegated {
		return false, "", "", ""
	}
	return true,
		strings.TrimSpace(info.ParentAgentID),
		strings.TrimSpace(info.ParentKeyType),
		strings.TrimSpace(info.ParentPublicKey)
}

func delegationDirForWriterPolicy(writerPolicyPath string) string {
	root := strings.TrimSpace(filepath.Dir(strings.TrimSpace(writerPolicyPath)))
	if root == "" || root == "." {
		return ""
	}
	return filepath.Join(root, "delegations")
}

func DelegationDirForWriterPolicy(writerPolicyPath string) string {
	return delegationDirForWriterPolicy(writerPolicyPath)
}

func revocationDirForWriterPolicy(writerPolicyPath string) string {
	root := strings.TrimSpace(filepath.Dir(strings.TrimSpace(writerPolicyPath)))
	if root == "" || root == "." {
		return ""
	}
	return filepath.Join(root, "revocations")
}

func RevocationDirForWriterPolicy(writerPolicyPath string) string {
	return revocationDirForWriterPolicy(writerPolicyPath)
}

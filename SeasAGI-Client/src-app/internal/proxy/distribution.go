package proxy

// DistributionPlan 表示 1:1 代理分配规划结果。
type DistributionPlan struct {
	Assignments map[string]string // providerName → proxyAddr
	Conflicts   []string          // 存在冲突的 provider 列表
}

// PlanDistribution 为 providers 规划 1:1 代理分配，避免多个 provider 共享同一代理。
// proxyPool: 可用代理地址列表
// providers: 需要分配代理的 provider 名称列表
func PlanDistribution(proxyPool []string, providers []string) DistributionPlan {
	assignments := make(map[string]string)
	used := make(map[string]bool)
	var conflicts []string

	for _, provider := range providers {
		assigned := false
		for _, addr := range proxyPool {
			if !used[addr] {
				assignments[provider] = addr
				used[addr] = true
				assigned = true
				break
			}
		}
		if !assigned {
			conflicts = append(conflicts, provider)
		}
	}

	return DistributionPlan{
		Assignments: assignments,
		Conflicts:   conflicts,
	}
}

// ValidateDistribution 验证分配结果是否满足 1:1 约束。
func ValidateDistribution(assignments map[string]string) bool {
	seen := make(map[string]string) // proxyAddr → providerName
	for provider, addr := range assignments {
		if existing, ok := seen[addr]; ok {
			// 同一代理被两个 provider 使用
			_ = existing
			return false
		}
		seen[addr] = provider
	}
	return true
}

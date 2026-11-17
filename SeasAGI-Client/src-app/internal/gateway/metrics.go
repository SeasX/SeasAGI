package gateway

// ComboRouteMetrics tracks route-level metrics per combo
type ComboRouteMetrics struct {
	ComboName        string  `json:"combo_name"`
	TotalRequests    int     `json:"total_requests"`
	TotalFallbacks   int     `json:"total_fallbacks"`
	Step1Success     int     `json:"step1_success"`
	LastStepFallback int     `json:"last_step_fallback"`
	Step1HitRate     float64 `json:"step1_hit_rate"`
	LastStepHitRate  float64 `json:"last_step_hit_rate"`
	AvgAttempts      float64 `json:"avg_attempts"`
	TaskType         string  `json:"task_type"`
}

// recordComboMetrics records routing metrics for a combo request
func (s *Service) recordComboMetrics(comboName string, numAttempts int, success bool) {
	if comboName == "" {
		return
	}
	s.metricsMu.Lock()
	defer s.metricsMu.Unlock()

	metrics, ok := s.comboMetrics[comboName]
	if !ok {
		metrics = &ComboRouteMetrics{
			ComboName: comboName,
		}
		s.comboMetrics[comboName] = metrics
	}

	metrics.TotalRequests++
	if numAttempts > 1 {
		metrics.TotalFallbacks++
	}
	if numAttempts == 1 && success {
		metrics.Step1Success++
	} else if success && numAttempts >= 2 {
		metrics.LastStepFallback++
	}

	// Update derived rates
	if metrics.TotalRequests > 0 {
		metrics.Step1HitRate = float64(metrics.Step1Success) / float64(metrics.TotalRequests)
		metrics.LastStepHitRate = float64(metrics.LastStepFallback) / float64(metrics.TotalRequests)
	}
	metrics.AvgAttempts = (metrics.AvgAttempts*float64(metrics.TotalRequests-1) + float64(numAttempts)) / float64(metrics.TotalRequests)
}

// GetComboRouteMetrics returns aggregated route metrics for all tracked combos
func (s *Service) GetComboRouteMetrics() []ComboRouteMetrics {
	s.metricsMu.RLock()
	defer s.metricsMu.RUnlock()

	result := make([]ComboRouteMetrics, 0, len(s.comboMetrics))
	for _, m := range s.comboMetrics {
		result = append(result, *m)
	}
	return result
}

// ResetComboRouteMetrics resets all combo route metrics
func (s *Service) ResetComboRouteMetrics() {
	s.metricsMu.Lock()
	defer s.metricsMu.Unlock()
	s.comboMetrics = make(map[string]*ComboRouteMetrics)
}

// SyncChannelConcurrencyLimits updates the concurrency limiter with the latest
// channel MaxConcurrent settings. Call this after channel configuration changes.
func (s *Service) SyncChannelConcurrencyLimits() {
	channels, _ := s.configSvc.ListChannels()
	for _, ch := range channels {
		s.concurrencyLimiter.SetLimit(ch.ChannelID, ch.MaxConcurrent)
	}
}

// GetConcurrencyInflight returns the current in-flight count for a channel.
func (s *Service) GetConcurrencyInflight(channelID string) int {
	return s.concurrencyLimiter.Inflight(channelID)
}

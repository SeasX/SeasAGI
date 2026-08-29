package plugin

// EmitHook 触发非阻塞事件（fire-and-forget）。
// handler 错误不会中断其他 handler。
func (r *Registry) EmitHook(event HookEvent, ctx *PluginContext) {
	r.mu.RLock()
	list := r.hookMap[event]
	r.mu.RUnlock()

	for _, reg := range list {
		if r.rateLimiter.IsRateLimited(reg.PluginName) {
			continue
		}
		// 非阻塞 hook 忽略返回值
		reg.Handler(ctx)
	}
}

// EmitHookBlocking 触发阻塞式事件，支持链式 body/metadata 传递。
// 返回第一个 blocked 的结果，或合并后的 body/metadata。
func (r *Registry) EmitHookBlocking(event HookEvent, ctx *PluginContext) *BlockingResult {
	r.mu.RLock()
	list := r.hookMap[event]
	r.mu.RUnlock()

	var mergedBody interface{}
	var mergedMetadata map[string]interface{}

	if ctx != nil {
		mergedBody = ctx.Body
		mergedMetadata = ctx.Metadata
	}

	for _, reg := range list {
		if r.rateLimiter.IsRateLimited(reg.PluginName) {
			continue
		}

		// 链式传递：每个 handler 看到前一个 handler 修改后的 body/metadata
		currentCtx := &PluginContext{
			RequestID:  ctx.RequestID,
			Body:       mergedBody,
			Model:      ctx.Model,
			Provider:   ctx.Provider,
			APIKeyInfo: ctx.APIKeyInfo,
			Metadata:   mergedMetadata,
		}

		result := reg.Handler(currentCtx)
		if result == nil {
			continue
		}

		// 合并 body
		if result.Body != nil {
			mergedBody = result.Body
		}

		// 合并 metadata
		if result.Metadata != nil {
			if mergedMetadata == nil {
				mergedMetadata = make(map[string]interface{})
			}
			for k, v := range result.Metadata {
				mergedMetadata[k] = v
			}
		}

		// 如果被阻塞，立即返回
		if result.Blocked {
			return &BlockingResult{
				Blocked:  true,
				Response: result.Response,
				Body:     mergedBody,
				Metadata: mergedMetadata,
			}
		}
	}

	return &BlockingResult{
		Body:     mergedBody,
		Metadata: mergedMetadata,
	}
}

// RunOnRequest 运行 onRequest hook（阻塞式）。
func (r *Registry) RunOnRequest(ctx *PluginContext) *BlockingResult {
	return r.EmitHookBlocking(HookOnRequest, ctx)
}

// RunOnResponse 运行 onResponse hook（链式响应修改）。
func (r *Registry) RunOnResponse(ctx *PluginContext, response interface{}) interface{} {
	r.mu.RLock()
	list := r.hookMap[HookOnResponse]
	r.mu.RUnlock()

	currentResponse := response

	for _, reg := range list {
		if r.rateLimiter.IsRateLimited(reg.PluginName) {
			continue
		}

		currentCtx := &PluginContext{
			RequestID:  ctx.RequestID,
			Body:       ctx.Body,
			Model:      ctx.Model,
			Provider:   ctx.Provider,
			APIKeyInfo: ctx.APIKeyInfo,
			Metadata:   ctx.Metadata,
		}

		result := reg.Handler(currentCtx)
		if result != nil && result.Response != nil {
			currentResponse = result.Response
		}
	}

	return currentResponse
}

// RunOnError 运行 onError hook（fire-and-forget）。
func (r *Registry) RunOnError(ctx *PluginContext) {
	r.EmitHook(HookOnError, ctx)
}

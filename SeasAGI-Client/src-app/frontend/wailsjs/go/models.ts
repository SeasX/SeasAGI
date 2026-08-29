export namespace auth {
	
	export class ActiveGrant {
	    grant_id: string;
	    grantor_user_id: string;
	    channel_id: string;
	    token_fingerprint: string;
	    granted_quota_usd: number;
	    used_quota_usd: number;
	    remaining_quota: number;
	    granted_tokens: number;
	    used_tokens: number;
	    remaining_tokens: number;
	    status: string;
	    expires_at: string;
	
	    static createFrom(source: any = {}) {
	        return new ActiveGrant(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.grant_id = source["grant_id"];
	        this.grantor_user_id = source["grantor_user_id"];
	        this.channel_id = source["channel_id"];
	        this.token_fingerprint = source["token_fingerprint"];
	        this.granted_quota_usd = source["granted_quota_usd"];
	        this.used_quota_usd = source["used_quota_usd"];
	        this.remaining_quota = source["remaining_quota"];
	        this.granted_tokens = source["granted_tokens"];
	        this.used_tokens = source["used_tokens"];
	        this.remaining_tokens = source["remaining_tokens"];
	        this.status = source["status"];
	        this.expires_at = source["expires_at"];
	    }
	}
	export class AuthInfo {
	    is_logged_in: boolean;
	    user_id?: string;
	    email?: string;
	
	    static createFrom(source: any = {}) {
	        return new AuthInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.is_logged_in = source["is_logged_in"];
	        this.user_id = source["user_id"];
	        this.email = source["email"];
	    }
	}
	export class RelayGateway {
	    gateway_id: string;
	    name: string;
	    host: string;
	    port: number;
	    region: string;
	    supports_federation: boolean;
	
	    static createFrom(source: any = {}) {
	        return new RelayGateway(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.gateway_id = source["gateway_id"];
	        this.name = source["name"];
	        this.host = source["host"];
	        this.port = source["port"];
	        this.region = source["region"];
	        this.supports_federation = source["supports_federation"];
	    }
	}
	export class CloudBilling {
	    plan_id: string;
	    plan_name: string;
	    price: number;
	    quota: number;
	    used_quota: number;
	    renewal_date: string;
	    relay_enabled: boolean;
	    relay_gateways: RelayGateway[];
	
	    static createFrom(source: any = {}) {
	        return new CloudBilling(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.plan_id = source["plan_id"];
	        this.plan_name = source["plan_name"];
	        this.price = source["price"];
	        this.quota = source["quota"];
	        this.used_quota = source["used_quota"];
	        this.renewal_date = source["renewal_date"];
	        this.relay_enabled = source["relay_enabled"];
	        this.relay_gateways = this.convertValues(source["relay_gateways"], RelayGateway);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class CloudCombo {
	    name: string;
	    models: string[];
	    description: string;
	    strategy: string;
	
	    static createFrom(source: any = {}) {
	        return new CloudCombo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.models = source["models"];
	        this.description = source["description"];
	        this.strategy = source["strategy"];
	    }
	}
	export class CloudComboTemplateStep {
	    channel_id?: string;
	    model: string;
	
	    static createFrom(source: any = {}) {
	        return new CloudComboTemplateStep(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.channel_id = source["channel_id"];
	        this.model = source["model"];
	    }
	}
	export class CloudComboTemplate {
	    name: string;
	    description: string;
	    tags: string[];
	    steps: CloudComboTemplateStep[];
	    models: string[];
	    strategy: string;
	    sticky_uses: number;
	
	    static createFrom(source: any = {}) {
	        return new CloudComboTemplate(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.description = source["description"];
	        this.tags = source["tags"];
	        this.steps = this.convertValues(source["steps"], CloudComboTemplateStep);
	        this.models = source["models"];
	        this.strategy = source["strategy"];
	        this.sticky_uses = source["sticky_uses"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	export class CloudModelStatsEntry {
	    model: string;
	    total_requests: number;
	    total_errors: number;
	    avg_latency_ms: number;
	    error_rate: number;
	
	    static createFrom(source: any = {}) {
	        return new CloudModelStatsEntry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.model = source["model"];
	        this.total_requests = source["total_requests"];
	        this.total_errors = source["total_errors"];
	        this.avg_latency_ms = source["avg_latency_ms"];
	        this.error_rate = source["error_rate"];
	    }
	}
	export class CloudPlan {
	    plan_id: string;
	    name: string;
	    description: string;
	    price: number;
	    monthly_quota: number;
	    max_rpm: number;
	    max_tpm: number;
	    sort_order: number;
	    relay_enabled: boolean;
	
	    static createFrom(source: any = {}) {
	        return new CloudPlan(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.plan_id = source["plan_id"];
	        this.name = source["name"];
	        this.description = source["description"];
	        this.price = source["price"];
	        this.monthly_quota = source["monthly_quota"];
	        this.max_rpm = source["max_rpm"];
	        this.max_tpm = source["max_tpm"];
	        this.sort_order = source["sort_order"];
	        this.relay_enabled = source["relay_enabled"];
	    }
	}
	export class CloudUsage {
	    month_requests: number;
	    month_input_tokens: number;
	    month_output_tokens: number;
	    total_cost_usd: number;
	
	    static createFrom(source: any = {}) {
	        return new CloudUsage(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.month_requests = source["month_requests"];
	        this.month_input_tokens = source["month_input_tokens"];
	        this.month_output_tokens = source["month_output_tokens"];
	        this.total_cost_usd = source["total_cost_usd"];
	    }
	}
	export class  {
	    channel_id?: string;
	    model: string;
	    step_role?: string;
	
	    static createFrom(source: any = {}) {
	        return new (source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.channel_id = source["channel_id"];
	        this.model = source["model"];
	        this.step_role = source["step_role"];
	    }
	}
	export class CloudUserCombo {
	    combo_id: string;
	    scope: string;
	    logical_name: string;
	    display_name: string;
	    description: string;
	    tags: string;
	    strategy: string;
	    sticky_uses: number;
	    quick_strategy: string;
	    task_profile: Record<string, any>;
	    steps: [];
	    status: string;
	    source: string;
	    version: number;
	    created_at: string;
	    updated_at: string;
	
	    static createFrom(source: any = {}) {
	        return new CloudUserCombo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.combo_id = source["combo_id"];
	        this.scope = source["scope"];
	        this.logical_name = source["logical_name"];
	        this.display_name = source["display_name"];
	        this.description = source["description"];
	        this.tags = source["tags"];
	        this.strategy = source["strategy"];
	        this.sticky_uses = source["sticky_uses"];
	        this.quick_strategy = source["quick_strategy"];
	        this.task_profile = source["task_profile"];
	        this.steps = this.convertValues(source["steps"], );
	        this.status = source["status"];
	        this.source = source["source"];
	        this.version = source["version"];
	        this.created_at = source["created_at"];
	        this.updated_at = source["updated_at"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

export namespace config {
	
	export class ChannelRateLimit {
	    rpm: number;
	    tpm: number;
	    min_interval_ms: number;
	    max_concurrent: number;
	
	    static createFrom(source: any = {}) {
	        return new ChannelRateLimit(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.rpm = source["rpm"];
	        this.tpm = source["tpm"];
	        this.min_interval_ms = source["min_interval_ms"];
	        this.max_concurrent = source["max_concurrent"];
	    }
	}
	export class RateLimitConfig {
	    enabled: boolean;
	    default_rpm: number;
	    default_tpm: number;
	    min_interval_ms: number;
	    max_concurrent: number;
	    max_wait_ms: number;
	    channel_overrides?: Record<string, ChannelRateLimit>;
	
	    static createFrom(source: any = {}) {
	        return new RateLimitConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.enabled = source["enabled"];
	        this.default_rpm = source["default_rpm"];
	        this.default_tpm = source["default_tpm"];
	        this.min_interval_ms = source["min_interval_ms"];
	        this.max_concurrent = source["max_concurrent"];
	        this.max_wait_ms = source["max_wait_ms"];
	        this.channel_overrides = this.convertValues(source["channel_overrides"], ChannelRateLimit, true);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class OptimizationConfig {
	    mode: string;
	    penalty_enabled: boolean;
	    penalty_decay_sec: number;
	    health_check_enabled: boolean;
	    health_check_sec: number;
	    health_max_failures: number;
	    cooldown_enabled: boolean;
	    cooldown_sec: number;
	    sticky_enabled: boolean;
	    sticky_ttl_sec: number;
	    preset_enabled: boolean;
	    default_preset: string;
	
	    static createFrom(source: any = {}) {
	        return new OptimizationConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.mode = source["mode"];
	        this.penalty_enabled = source["penalty_enabled"];
	        this.penalty_decay_sec = source["penalty_decay_sec"];
	        this.health_check_enabled = source["health_check_enabled"];
	        this.health_check_sec = source["health_check_sec"];
	        this.health_max_failures = source["health_max_failures"];
	        this.cooldown_enabled = source["cooldown_enabled"];
	        this.cooldown_sec = source["cooldown_sec"];
	        this.sticky_enabled = source["sticky_enabled"];
	        this.sticky_ttl_sec = source["sticky_ttl_sec"];
	        this.preset_enabled = source["preset_enabled"];
	        this.default_preset = source["default_preset"];
	    }
	}
	export class OAuthProviderConfig {
	    provider_name: string;
	    client_id: string;
	    client_secret?: string;
	
	    static createFrom(source: any = {}) {
	        return new OAuthProviderConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.provider_name = source["provider_name"];
	        this.client_id = source["client_id"];
	        this.client_secret = source["client_secret"];
	    }
	}
	export class CandidateProvider {
	    channel_id: string;
	    model: string;
	    priority: number;
	    health_status?: string;
	
	    static createFrom(source: any = {}) {
	        return new CandidateProvider(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.channel_id = source["channel_id"];
	        this.model = source["model"];
	        this.priority = source["priority"];
	        this.health_status = source["health_status"];
	    }
	}
	export class ModelComboStep {
	    channel_id?: string;
	    model: string;
	    step_role?: string;
	    providers?: CandidateProvider[];
	    channels?: string[];
	    selection_policy?: string;
	    allow_provider_fallback?: boolean;
	    allow_cross_provider_fallback?: boolean;
	
	    static createFrom(source: any = {}) {
	        return new ModelComboStep(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.channel_id = source["channel_id"];
	        this.model = source["model"];
	        this.step_role = source["step_role"];
	        this.providers = this.convertValues(source["providers"], CandidateProvider);
	        this.channels = source["channels"];
	        this.selection_policy = source["selection_policy"];
	        this.allow_provider_fallback = source["allow_provider_fallback"];
	        this.allow_cross_provider_fallback = source["allow_cross_provider_fallback"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class ModelCombo {
	    combo_id?: string;
	    name: string;
	    logical_name?: string;
	    display_name?: string;
	    description?: string;
	    tags?: string[];
	    steps?: ModelComboStep[];
	    models?: string[];
	    strategy: string;
	    sticky_uses: number;
	    quick_strategy?: string;
	    task_profile?: Record<string, any>;
	    status?: string;
	    source?: string;
	    version?: number;
	
	    static createFrom(source: any = {}) {
	        return new ModelCombo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.combo_id = source["combo_id"];
	        this.name = source["name"];
	        this.logical_name = source["logical_name"];
	        this.display_name = source["display_name"];
	        this.description = source["description"];
	        this.tags = source["tags"];
	        this.steps = this.convertValues(source["steps"], ModelComboStep);
	        this.models = source["models"];
	        this.strategy = source["strategy"];
	        this.sticky_uses = source["sticky_uses"];
	        this.quick_strategy = source["quick_strategy"];
	        this.task_profile = source["task_profile"];
	        this.status = source["status"];
	        this.source = source["source"];
	        this.version = source["version"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class AppConfig {
	    listen_port: number;
	    default_model: string;
	    default_channel_id: string;
	    routing_strategy: string;
	    sticky_channel_use: number;
	    auto_launch: boolean;
	    auto_update: boolean;
	    log_retention_days: number;
	    analytics_enabled: boolean;
	    locale: string;
	    rtk_enabled: boolean;
	    rtk_max_output_chars: number;
	    caveman_enabled: boolean;
	    caveman_style: string;
	    model_combos?: ModelCombo[];
	    combo_templates?: ModelCombo[];
	    oauth_providers?: OAuthProviderConfig[];
	    optimizations?: OptimizationConfig;
	    platform_api_base_url?: string;
	    default_combo_name?: string;
	    rate_limit?: RateLimitConfig;
	    selected_grant_id?: string;
	    selected_grant_relay_url?: string;
	
	    static createFrom(source: any = {}) {
	        return new AppConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.listen_port = source["listen_port"];
	        this.default_model = source["default_model"];
	        this.default_channel_id = source["default_channel_id"];
	        this.routing_strategy = source["routing_strategy"];
	        this.sticky_channel_use = source["sticky_channel_use"];
	        this.auto_launch = source["auto_launch"];
	        this.auto_update = source["auto_update"];
	        this.log_retention_days = source["log_retention_days"];
	        this.analytics_enabled = source["analytics_enabled"];
	        this.locale = source["locale"];
	        this.rtk_enabled = source["rtk_enabled"];
	        this.rtk_max_output_chars = source["rtk_max_output_chars"];
	        this.caveman_enabled = source["caveman_enabled"];
	        this.caveman_style = source["caveman_style"];
	        this.model_combos = this.convertValues(source["model_combos"], ModelCombo);
	        this.combo_templates = this.convertValues(source["combo_templates"], ModelCombo);
	        this.oauth_providers = this.convertValues(source["oauth_providers"], OAuthProviderConfig);
	        this.optimizations = this.convertValues(source["optimizations"], OptimizationConfig);
	        this.platform_api_base_url = source["platform_api_base_url"];
	        this.default_combo_name = source["default_combo_name"];
	        this.rate_limit = this.convertValues(source["rate_limit"], RateLimitConfig);
	        this.selected_grant_id = source["selected_grant_id"];
	        this.selected_grant_relay_url = source["selected_grant_relay_url"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	export class RetryConfig {
	    max_retries: number;
	    initial_delay: string;
	    max_delay: string;
	    retryable_only: boolean;
	
	    static createFrom(source: any = {}) {
	        return new RetryConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.max_retries = source["max_retries"];
	        this.initial_delay = source["initial_delay"];
	        this.max_delay = source["max_delay"];
	        this.retryable_only = source["retryable_only"];
	    }
	}
	export class Channel {
	    channel_id: string;
	    channel_type: string;
	    provider_type: string;
	    display_name: string;
	    base_url: string;
	    enabled: boolean;
	    health_status: string;
	    source?: string;
	    provider_specific_config: Record<string, string>;
	    supported_modalities?: string[];
	    models?: string[];
	    api_key?: string;
	    api_keys?: string[];
	    retry_config?: RetryConfig;
	    weight?: number;
	    priority?: number;
	    max_concurrent?: number;
	    oauth_refresh_token?: string;
	    oauth_client_id?: string;
	    oauth_client_secret?: string;
	    oauth_token_url?: string;
	
	    static createFrom(source: any = {}) {
	        return new Channel(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.channel_id = source["channel_id"];
	        this.channel_type = source["channel_type"];
	        this.provider_type = source["provider_type"];
	        this.display_name = source["display_name"];
	        this.base_url = source["base_url"];
	        this.enabled = source["enabled"];
	        this.health_status = source["health_status"];
	        this.source = source["source"];
	        this.provider_specific_config = source["provider_specific_config"];
	        this.supported_modalities = source["supported_modalities"];
	        this.models = source["models"];
	        this.api_key = source["api_key"];
	        this.api_keys = source["api_keys"];
	        this.retry_config = this.convertValues(source["retry_config"], RetryConfig);
	        this.weight = source["weight"];
	        this.priority = source["priority"];
	        this.max_concurrent = source["max_concurrent"];
	        this.oauth_refresh_token = source["oauth_refresh_token"];
	        this.oauth_client_id = source["oauth_client_id"];
	        this.oauth_client_secret = source["oauth_client_secret"];
	        this.oauth_token_url = source["oauth_token_url"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	
	
	
	
	

}

export namespace discovery {
	
	export class DiscoveredModel {
	    channel_id: string;
	    model_id: string;
	    model_name: string;
	    discovered_at: string;
	
	    static createFrom(source: any = {}) {
	        return new DiscoveredModel(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.channel_id = source["channel_id"];
	        this.model_id = source["model_id"];
	        this.model_name = source["model_name"];
	        this.discovered_at = source["discovered_at"];
	    }
	}

}

export namespace gateway {
	
	export class ComboRouteMetrics {
	    combo_name: string;
	    total_requests: number;
	    total_fallbacks: number;
	    step1_success: number;
	    last_step_fallback: number;
	    step1_hit_rate: number;
	    last_step_hit_rate: number;
	    avg_attempts: number;
	    task_type: string;
	
	    static createFrom(source: any = {}) {
	        return new ComboRouteMetrics(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.combo_name = source["combo_name"];
	        this.total_requests = source["total_requests"];
	        this.total_fallbacks = source["total_fallbacks"];
	        this.step1_success = source["step1_success"];
	        this.last_step_fallback = source["last_step_fallback"];
	        this.step1_hit_rate = source["step1_hit_rate"];
	        this.last_step_hit_rate = source["last_step_hit_rate"];
	        this.avg_attempts = source["avg_attempts"];
	        this.task_type = source["task_type"];
	    }
	}

}

export namespace logs {
	
	export class RouteStep {
	    channel_id: string;
	    upstream_model: string;
	    status: string;
	    error?: string;
	    step_role?: string;
	    within_step?: boolean;
	    max_price?: number;
	    max_latency_ms?: number;
	
	    static createFrom(source: any = {}) {
	        return new RouteStep(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.channel_id = source["channel_id"];
	        this.upstream_model = source["upstream_model"];
	        this.status = source["status"];
	        this.error = source["error"];
	        this.step_role = source["step_role"];
	        this.within_step = source["within_step"];
	        this.max_price = source["max_price"];
	        this.max_latency_ms = source["max_latency_ms"];
	    }
	}
	export class RequestLog {
	    request_id: string;
	    created_at: string;
	    logical_model_name: string;
	    channel_id: string;
	    upstream_model: string;
	    route_trace: string;
	    route_steps?: RouteStep[];
	    status: string;
	    duration_ms: number;
	    error_code?: string;
	    error_message?: string;
	    applied_constraints?: string;
	
	    static createFrom(source: any = {}) {
	        return new RequestLog(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.request_id = source["request_id"];
	        this.created_at = source["created_at"];
	        this.logical_model_name = source["logical_model_name"];
	        this.channel_id = source["channel_id"];
	        this.upstream_model = source["upstream_model"];
	        this.route_trace = source["route_trace"];
	        this.route_steps = this.convertValues(source["route_steps"], RouteStep);
	        this.status = source["status"];
	        this.duration_ms = source["duration_ms"];
	        this.error_code = source["error_code"];
	        this.error_message = source["error_message"];
	        this.applied_constraints = source["applied_constraints"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

export namespace mcp {
	
	export class MCPServer {
	    name: string;
	    command: string;
	    args?: string[];
	    env?: Record<string, string>;
	    transport: string;
	    url?: string;
	    enabled: boolean;
	    apps: string[];
	    description?: string;
	
	    static createFrom(source: any = {}) {
	        return new MCPServer(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.command = source["command"];
	        this.args = source["args"];
	        this.env = source["env"];
	        this.transport = source["transport"];
	        this.url = source["url"];
	        this.enabled = source["enabled"];
	        this.apps = source["apps"];
	        this.description = source["description"];
	    }
	}

}

export namespace mitm {
	
	export class Manager {
	
	
	    static createFrom(source: any = {}) {
	        return new Manager(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	
	    }
	}

}

export namespace presets {
	
	export class ProviderPreset {
	    name: string;
	    provider_type: string;
	    base_url: string;
	    display_name: string;
	    region?: string;
	    category: string;
	    models?: string[];
	    extra_config?: Record<string, string>;
	
	    static createFrom(source: any = {}) {
	        return new ProviderPreset(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.provider_type = source["provider_type"];
	        this.base_url = source["base_url"];
	        this.display_name = source["display_name"];
	        this.region = source["region"];
	        this.category = source["category"];
	        this.models = source["models"];
	        this.extra_config = source["extra_config"];
	    }
	}

}

export namespace prompts {
	
	export class PromptPreset {
	    name: string;
	    content: string;
	    target_app: string;
	    is_default: boolean;
	    description?: string;
	
	    static createFrom(source: any = {}) {
	        return new PromptPreset(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.content = source["content"];
	        this.target_app = source["target_app"];
	        this.is_default = source["is_default"];
	        this.description = source["description"];
	    }
	}

}

export namespace sessions {
	
	export class Message {
	    role: string;
	    content: string;
	    // Go type: time
	    timestamp: any;
	    model?: string;
	
	    static createFrom(source: any = {}) {
	        return new Message(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.role = source["role"];
	        this.content = source["content"];
	        this.timestamp = this.convertValues(source["timestamp"], null);
	        this.model = source["model"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class Session {
	    id: string;
	    app: string;
	    title: string;
	    // Go type: time
	    created_at: any;
	    // Go type: time
	    updated_at: any;
	    model?: string;
	    message_count: number;
	    path: string;
	
	    static createFrom(source: any = {}) {
	        return new Session(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.app = source["app"];
	        this.title = source["title"];
	        this.created_at = this.convertValues(source["created_at"], null);
	        this.updated_at = this.convertValues(source["updated_at"], null);
	        this.model = source["model"];
	        this.message_count = source["message_count"];
	        this.path = source["path"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

export namespace skills {
	
	export class Skill {
	    name: string;
	    source: string;
	    source_url?: string;
	    install_path: string;
	    install_type: string;
	    description?: string;
	    enabled: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Skill(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.source = source["source"];
	        this.source_url = source["source_url"];
	        this.install_path = source["install_path"];
	        this.install_type = source["install_type"];
	        this.description = source["description"];
	        this.enabled = source["enabled"];
	    }
	}

}

export namespace sync {
	
	export class SyncConfig {
	    provider: string;
	    remote_path: string;
	    webdav_url?: string;
	    username?: string;
	    password?: string;
	    auto_sync: boolean;
	    interval: number;
	
	    static createFrom(source: any = {}) {
	        return new SyncConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.provider = source["provider"];
	        this.remote_path = source["remote_path"];
	        this.webdav_url = source["webdav_url"];
	        this.username = source["username"];
	        this.password = source["password"];
	        this.auto_sync = source["auto_sync"];
	        this.interval = source["interval"];
	    }
	}
	export class SyncStatus {
	    last_sync_time: string;
	    status: string;
	    error?: string;
	
	    static createFrom(source: any = {}) {
	        return new SyncStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.last_sync_time = source["last_sync_time"];
	        this.status = source["status"];
	        this.error = source["error"];
	    }
	}

}

export namespace usage {
	
	export class ChannelUsage {
	    requests: number;
	    input_tokens: number;
	    output_tokens: number;
	    cost_usd: number;
	
	    static createFrom(source: any = {}) {
	        return new ChannelUsage(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.requests = source["requests"];
	        this.input_tokens = source["input_tokens"];
	        this.output_tokens = source["output_tokens"];
	        this.cost_usd = source["cost_usd"];
	    }
	}
	export class DailyUsage {
	    date: string;
	    total_requests: number;
	    total_input_tokens: number;
	    total_output_tokens: number;
	    total_cost_usd: number;
	    by_channel: Record<string, ChannelUsage>;
	
	    static createFrom(source: any = {}) {
	        return new DailyUsage(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.date = source["date"];
	        this.total_requests = source["total_requests"];
	        this.total_input_tokens = source["total_input_tokens"];
	        this.total_output_tokens = source["total_output_tokens"];
	        this.total_cost_usd = source["total_cost_usd"];
	        this.by_channel = this.convertValues(source["by_channel"], ChannelUsage, true);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class ModelPricing {
	    model: string;
	    input_price_per_1m: number;
	    output_price_per_1m: number;
	
	    static createFrom(source: any = {}) {
	        return new ModelPricing(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.model = source["model"];
	        this.input_price_per_1m = source["input_price_per_1m"];
	        this.output_price_per_1m = source["output_price_per_1m"];
	    }
	}
	export class ModelStatsEntry {
	    model: string;
	    total_requests: number;
	    total_errors: number;
	    avg_latency_ms: number;
	    error_rate: number;
	
	    static createFrom(source: any = {}) {
	        return new ModelStatsEntry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.model = source["model"];
	        this.total_requests = source["total_requests"];
	        this.total_errors = source["total_errors"];
	        this.avg_latency_ms = source["avg_latency_ms"];
	        this.error_rate = source["error_rate"];
	    }
	}
	export class UsageDetail {
	    ChannelID: string;
	    ChannelName: string;
	    Model: string;
	    InputTokens: number;
	    OutputTokens: number;
	    CacheReadTokens: number;
	    CacheWriteTokens: number;
	    UncachedInputTokens: number;
	    ReasoningTokens: number;
	    NonReasoningOutput: number;
	    UnclassifiedTokens: number;
	    TTFTMs: number;
	    LatencyMs: number;
	    ServiceTier: string;
	    RateLimitRemaining: number;
	    RateLimitLimit: number;
	    RateLimitReset: number;
	
	    static createFrom(source: any = {}) {
	        return new UsageDetail(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ChannelID = source["ChannelID"];
	        this.ChannelName = source["ChannelName"];
	        this.Model = source["Model"];
	        this.InputTokens = source["InputTokens"];
	        this.OutputTokens = source["OutputTokens"];
	        this.CacheReadTokens = source["CacheReadTokens"];
	        this.CacheWriteTokens = source["CacheWriteTokens"];
	        this.UncachedInputTokens = source["UncachedInputTokens"];
	        this.ReasoningTokens = source["ReasoningTokens"];
	        this.NonReasoningOutput = source["NonReasoningOutput"];
	        this.UnclassifiedTokens = source["UnclassifiedTokens"];
	        this.TTFTMs = source["TTFTMs"];
	        this.LatencyMs = source["LatencyMs"];
	        this.ServiceTier = source["ServiceTier"];
	        this.RateLimitRemaining = source["RateLimitRemaining"];
	        this.RateLimitLimit = source["RateLimitLimit"];
	        this.RateLimitReset = source["RateLimitReset"];
	    }
	}
	export class UsageRecord {
	    timestamp: string;
	    channel_id: string;
	    channel_name: string;
	    model: string;
	    request_count: number;
	    input_tokens: number;
	    output_tokens: number;
	    cost_usd: number;
	    cache_read_tokens?: number;
	    cache_write_tokens?: number;
	    uncached_input_tokens?: number;
	    reasoning_tokens?: number;
	    non_reasoning_output?: number;
	    unclassified_tokens?: number;
	    token_quality?: string;
	    ttft_ms?: number;
	    latency_ms?: number;
	    service_tier?: string;
	    rate_limit_remaining?: number;
	    rate_limit_limit?: number;
	    rate_limit_reset?: number;
	
	    static createFrom(source: any = {}) {
	        return new UsageRecord(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.timestamp = source["timestamp"];
	        this.channel_id = source["channel_id"];
	        this.channel_name = source["channel_name"];
	        this.model = source["model"];
	        this.request_count = source["request_count"];
	        this.input_tokens = source["input_tokens"];
	        this.output_tokens = source["output_tokens"];
	        this.cost_usd = source["cost_usd"];
	        this.cache_read_tokens = source["cache_read_tokens"];
	        this.cache_write_tokens = source["cache_write_tokens"];
	        this.uncached_input_tokens = source["uncached_input_tokens"];
	        this.reasoning_tokens = source["reasoning_tokens"];
	        this.non_reasoning_output = source["non_reasoning_output"];
	        this.unclassified_tokens = source["unclassified_tokens"];
	        this.token_quality = source["token_quality"];
	        this.ttft_ms = source["ttft_ms"];
	        this.latency_ms = source["latency_ms"];
	        this.service_tier = source["service_tier"];
	        this.rate_limit_remaining = source["rate_limit_remaining"];
	        this.rate_limit_limit = source["rate_limit_limit"];
	        this.rate_limit_reset = source["rate_limit_reset"];
	    }
	}
	export class UsageSummary {
	    month_requests: number;
	    month_input_tokens: number;
	    month_output_tokens: number;
	    month_cost_usd: number;
	
	    static createFrom(source: any = {}) {
	        return new UsageSummary(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.month_requests = source["month_requests"];
	        this.month_input_tokens = source["month_input_tokens"];
	        this.month_output_tokens = source["month_output_tokens"];
	        this.month_cost_usd = source["month_cost_usd"];
	    }
	}

}


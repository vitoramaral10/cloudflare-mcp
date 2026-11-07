package cloudflare

import "strings"

// glossary maps common Portuguese terms to Cloudflare vocabulary. The expanded query
// ("bloquear país" → "bloquear país block country") feeds the keyword search, because the
// spec descriptions are in English.
var glossary = map[string]string{
	"redirecionar": "redirect", "redirecionamento": "redirect", "redirecione": "redirect", "redireciona": "redirect",
	"cabeçalho": "header", "cabecalho": "header", "cabeçalhos": "headers",
	"bloquear": "block", "bloqueio": "block", "barrar": "block",
	"país": "country", "pais": "country", "países": "country",
	"túnel": "tunnel", "tunel": "tunnel", "túneis": "tunnels", "tuneis": "tunnels",
	"registro": "record", "registros": "records",
	"certificado": "certificate", "certificados": "certificates",
	"origem": "origin", "servidor": "origin server",
	"domínio": "domain zone", "dominio": "domain zone", "domínios": "domains zones",
	"subdomínio": "subdomain", "subdominio": "subdomain",
	"apontar": "point dns record", "regra": "rule", "regras": "rules",
	"visitas": "visits", "visitantes": "visits", "acessos": "visits",
	"tráfego": "traffic", "trafego": "traffic",
	"requisições": "requests", "requisicoes": "requests", "limitar": "limit rate",
	"fila": "queue waiting room", "espera": "waiting",
	"limpar": "purge clear", "apagar": "delete", "excluir": "delete", "remover": "delete",
	"criar": "create", "adicionar": "add create", "novo": "new create", "listar": "list",
	"mudar": "edit update", "alterar": "edit update", "editar": "edit",
	"segredo": "secret", "secreta": "secret", "secreto": "secret", "variável": "variable", "variavel": "variable",
	"chave": "key", "arquivo": "file", "arquivos": "files", "cachear": "cache",
	"proteger": "protect", "entrar": "access login", "login": "access login",
	"encaminhar": "forward routing", "e-mail": "email", "emails": "email",
	"pausar": "pause", "captcha": "turnstile challenge visitor",
	"reescrever": "rewrite transform", "consulta": "query", "implantação": "deployment",
	"deploys": "deployments", "deploy": "deployment", "saúde": "health",
	"zona": "zone", "conta": "account", "forçar": "force always", "forcar": "force always",
	"sempre": "always", "próprio": "custom", "proprio": "custom", "customizado": "custom",
	"personalizado": "custom", "guardar": "storage",
	"publicar": "publish expose hostname", "lotar": "capacity", "robô": "bot", "robo": "bot", "robôs": "bot",
	"site": "zone", "sala": "room", "senha": "password access login", "painel": "dashboard",
}

func expandQuery(q string) string {
	var extra []string
	for _, t := range tokens(q) {
		if en, ok := glossary[t]; ok {
			extra = append(extra, en)
		}
	}
	if len(extra) == 0 {
		return q
	}
	return q + " " + strings.Join(extra, " ")
}

// guide ties an intent to operations and a usage tip. It exists for the cases where
// Cloudflare solves the task with a generic mechanism (ruleset, list, GraphQL) whose
// description does not mention the task.
type guide struct {
	when [][]string // fires if ALL terms of some group appear in the expanded query
	ops  []string
	tip  string
}

var guides = []guide{
	{when: [][]string{{"redirect"}}, ops: []string{"rulesets_zone_entrypoint_update", "rulesets_zone_rules_create", "rules_lists_items_create", "page_rules_create"},
		tip: `Redirecting a domain/URL: Single Redirect = rule in the zone's "http_request_dynamic_redirect" phase (rulesets_zone_entrypoint_get to read, rulesets_zone_entrypoint_update or rulesets_zone_rules_create to write), action "redirect", action_parameters.from_value {target_url:{expression:'concat("https://target.com", http.request.uri.path)'}, status_code:301, preserve_query_string:true}. Many fixed redirects: Bulk Redirects = list of kind "redirect" (rules_lists_create, rules_lists_items_create) + rule in the account's "http_request_redirect" phase.`},
	{when: [][]string{{"waf"}, {"firewall", "rule"}, {"block", "country"}, {"block", "ip"}, {"block", "bot"}, {"block", "request"}, {"block", "traffic"}, {"challenge", "visitor"}, {"protect", "attack"}}, ops: []string{"rulesets_zone_entrypoint_update", "rulesets_zone_rules_create", "rulesets_zone_entrypoint_get", "firewall_access_rules_create"},
		tip: `Blocking/challenging traffic: WAF custom rule in the zone's "http_request_firewall_custom" phase (rulesets_zone_entrypoint_get, then rulesets_zone_rules_create on the ruleset id or rulesets_zone_entrypoint_update with the full list). Example expression: (ip.src.country in {"CN" "RU"}) or (http.request.uri.path eq "/login" and not ip.src in {1.2.3.4}); action "block" or "managed_challenge". Simple blocking by IP, range, ASN or country also works via firewall_access_rules_create (mode block).`},
	{when: [][]string{{"rate", "limit"}, {"rate", "limiting"}}, ops: []string{"rulesets_zone_entrypoint_update", "rulesets_zone_rules_create", "rate_limits_create"},
		tip: `Current rate limiting = rule in the zone's "http_ratelimit" phase (rulesets_zone_rules_create / rulesets_zone_entrypoint_update) with ratelimit {characteristics:["ip.src","cf.colo.id"], period:60, requests_per_period:100, mitigation_timeout:600}. rate_limits_* is the legacy API.`},
	{when: [][]string{{"header"}, {"headers"}, {"rewrite"}, {"transform"}}, ops: []string{"rulesets_zone_entrypoint_update", "rulesets_zone_rules_create", "managed_transforms_update", "url_normalization_update"},
		tip: `Transform Rules = ruleset rules on the zone: response header in the "http_response_headers_transform" phase, request header in "http_request_late_transform", URL rewrite in "http_request_transform" (action "rewrite", action_parameters.uri.path / headers). managed_transforms_update turns on the prebuilt transforms.`},
	{when: [][]string{{"cache", "rule"}, {"cache", "rules"}, {"cache", "bypass"}}, ops: []string{"rulesets_zone_entrypoint_update", "rulesets_zone_rules_create", "page_rules_create"},
		tip: `Cache Rules = rules in the zone's "http_request_cache_settings" phase, action "set_cache_settings", e.g. action_parameters {cache:false} to skip caching /api/*.`},
	{when: [][]string{{"purge", "cache"}, {"purge", "everything"}, {"purge", "file"}, {"cache", "file"}, {"cache", "delete"}, {"cache", "clear"}}, ops: []string{"cache_purge"},
		tip: `Purging the zone cache: cache_purge with files:[URLs], prefixes, hosts, tags or purge_everything:true.`},
	{when: [][]string{{"analytics"}, {"traffic", "stats"}, {"visits"}, {"page", "views"}, {"bandwidth"}}, ops: []string{"analytics_dashboard_get"},
		tip: `Zone traffic comes from the GraphQL API (analytics_dashboard_get is the legacy REST one). Use api_request {method:"POST", path:"/graphql", body:{query:"{viewer{zones(filter:{zoneTag:\"<zone_id>\"}){httpRequests1dGroups(limit:7, filter:{date_geq:\"2026-01-01\"}){dimensions{date} sum{requests pageViews bytes threats} uniq{uniques}}}}}"}}; for hourly data use httpRequests1hGroups with datetime_geq.`},
	{when: [][]string{{"ssl", "mode"}, {"ssl", "strict"}, {"ssl", "flexible"}, {"always", "https"}, {"force", "https"}, {"tls", "version"}}, ops: []string{"zones_settings_edit", "zones_settings_get"},
		tip: `Zone settings: zones_settings_edit with setting_id "ssl" and value "off"|"flexible"|"full"|"strict"; "always_use_https" value "on"; "min_tls_version" "1.2".`},
	{when: [][]string{{"dns", "record"}, {"cname"}}, ops: []string{"dns_records_create", "dns_records_edit", "dns_records_list"},
		tip: `DNS: dns_records_list (name.exact filter) to find the id, dns_records_create / dns_records_edit to write; proxied:true routes through Cloudflare.`},
	{when: [][]string{{"tunnel", "publish"}, {"tunnel", "service"}, {"tunnel", "hostname"}, {"tunnel", "expose"}}, ops: []string{"tunnels_config_get", "tunnels_config_update"},
		tip: `Publishing a hostname on a tunnel: prefer the tunnel_route_add tool (inserts before the catch-all and creates the CNAME). tunnels_config_update replaces the whole config.`},
	{when: [][]string{{"access", "application"}, {"access", "app"}, {"access", "policy"}, {"access", "login"}, {"zero", "trust", "access"}}, ops: []string{"zero_trust_access_applications_create", "zero_trust_access_policies_list", "zero_trust_access_applications_list"},
		tip: `Cloudflare Access: zero_trust_access_applications_create {type:"self_hosted", domain, name, policies:[{id, precedence}]}; reusable policies in zero_trust_access_policies_list.`},
	{when: [][]string{{"new", "domain"}, {"new", "zone"}, {"add", "domain", "account"}, {"onboard", "domain"}}, ops: []string{"zones_create"},
		tip: `New domain: zones_create {name, account:{id}, type:"full"}; then switch the nameservers at the registrar.`},
	{when: [][]string{{"pause", "zone"}, {"pause", "domain"}, {"pause", "cloudflare"}}, ops: []string{"zones_edit"},
		tip: `Pausing Cloudflare on a domain: zones_edit {paused:true}.`},
	{when: [][]string{{"email", "forward"}, {"email", "routing"}, {"email", "rules"}}, ops: []string{"email_routing_rules_create", "email_routing_addresses_create", "email_routing_rules_list"},
		tip: `Email Routing: the destination must be verified (email_routing_addresses_create, at account level); then email_routing_rules_create {matchers:[{type:"literal", field:"to", value:"me@example.com"}], actions:[{type:"forward", value:["inbox@example.net"]}]}.`},
	{when: [][]string{{"waiting", "room"}, {"queue", "capacity"}}, ops: []string{"waiting_rooms_create"}, tip: ""},
	{when: [][]string{{"health", "check"}, {"healthcheck"}, {"healthchecks"}}, ops: []string{"healthchecks_create", "healthchecks_list"}, tip: ""},
	{when: [][]string{{"api", "token"}}, ops: []string{"user_tokens_create", "accounts_tokens_create"},
		tip: `Tokens: user_tokens_create (user token) or accounts_tokens_create (account token); permissions in accounts_tokens_permission_groups_list.`},
}

// matchGuides returns the guides whose intent appears in the (already expanded) query.
func matchGuides(expanded string) []guide {
	have := map[string]bool{}
	for _, t := range tokens(expanded) {
		have[t] = true
		have[stem(t)] = true
	}
	var out []guide
	for _, g := range guides {
		for _, group := range g.when {
			all := true
			for _, t := range group {
				all = all && have[t]
			}
			if all {
				out = append(out, g)
				break
			}
		}
	}
	return out
}

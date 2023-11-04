package outboundgroup

import (
	"errors"
	"fmt"
	"strings"

	"github.com/Dreamacro/clash/adapter/outbound"
	"github.com/Dreamacro/clash/adapter/provider"
	"github.com/Dreamacro/clash/common/structure"
	"github.com/Dreamacro/clash/common/utils"
	C "github.com/Dreamacro/clash/constant"
	types "github.com/Dreamacro/clash/constant/provider"
)

var (
	errFormat            = errors.New("format error")
	errType              = errors.New("unsupported type")
	errMissProxy         = errors.New("`use` or `proxies` missing")
	errDuplicateProvider = errors.New("duplicate provider name")
)

type GroupCommonOption struct {
	outbound.BasicOption
	Name           string   `group:"name"`
	Type           string   `group:"type"`
	Proxies        []string `group:"proxies,omitempty"`
	Use            []string `group:"use,omitempty"`
	URL            string   `group:"url,omitempty"`
	Interval       int      `group:"interval,omitempty"`
	Lazy           bool     `group:"lazy,omitempty"`
	DisableUDP     bool     `group:"disable-udp,omitempty"`
	Filter         string   `group:"filter,omitempty"`
	ExcludeFilter  string   `group:"exclude-filter,omitempty"`
	ExcludeType    string   `group:"exclude-type,omitempty"`
	ExpectedStatus string   `group:"expected-status,omitempty"`
}

func ParseProxyGroup(config map[string]any, proxyMap map[string]C.Proxy, providersMap map[string]types.ProxyProvider) (C.ProxyAdapter, error) {
	decoder := structure.NewDecoder(structure.Option{TagName: "group", WeaklyTypedInput: true})

	groupOption := &GroupCommonOption{
		Lazy: true,
	}
	if err := decoder.Decode(config, groupOption); err != nil {
		return nil, errFormat
	}

	if groupOption.Type == "" || groupOption.Name == "" {
		return nil, errFormat
	}

	groupName := groupOption.Name

	providers := []types.ProxyProvider{}

	if len(groupOption.Proxies) == 0 && len(groupOption.Use) == 0 {
		return nil, fmt.Errorf("%s: %w", groupName, errMissProxy)
	}

	expectedStatus, err := utils.NewIntRanges[uint16](groupOption.ExpectedStatus)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", groupName, err)
	}

	status := strings.TrimSpace(groupOption.ExpectedStatus)
	if status == "" {
		status = "*"
	}
	groupOption.ExpectedStatus = status
	testUrl := groupOption.URL

	if len(groupOption.Proxies) != 0 {
		ps, err := getProxies(proxyMap, groupOption.Proxies)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", groupName, err)
		}

		if _, ok := providersMap[groupName]; ok {
			return nil, fmt.Errorf("%s: %w", groupName, errDuplicateProvider)
		}

		var url string
		var interval uint

		// select don't need health check
		if groupOption.Type != "select" && groupOption.Type != "relay" {
			if groupOption.URL == "" {
				groupOption.URL = "https://cp.cloudflare.com/generate_204"
			}

			if groupOption.Interval == 0 {
				groupOption.Interval = 300
			}

			url = groupOption.URL
			interval = uint(groupOption.Interval)
		}

		hc := provider.NewHealthCheck(ps, url, interval, true, expectedStatus)
		pd, err := provider.NewCompatibleProvider(groupName, ps, hc)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", groupName, err)
		}

		providers = append(providers, pd)
		providersMap[groupName] = pd
	}

	if len(groupOption.Use) != 0 {
		list, err := getProviders(providersMap, groupOption.Use)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", groupName, err)
		}

		// different proxy groups use different test URL
		addTestUrlToProviders(list, testUrl, expectedStatus, groupOption.Filter, uint(groupOption.Interval))

		providers = append(providers, list...)
	} else {
		groupOption.Filter = ""
	}

	var group C.ProxyAdapter
	switch groupOption.Type {
	case "url-test":
		opts := parseURLTestOption(config)
		group = NewURLTest(groupOption, providers, opts...)
	case "select":
		group = NewSelector(groupOption, providers)
	case "fallback":
		group = NewFallback(groupOption, providers)
	case "load-balance":
		strategy := parseStrategy(config)
		return NewLoadBalance(groupOption, providers, strategy)
	case "relay":
		group = NewRelay(groupOption, providers)
	default:
		return nil, fmt.Errorf("%w: %s", errType, groupOption.Type)
	}

	return group, nil
}

func getProxies(mapping map[string]C.Proxy, list []string) ([]C.Proxy, error) {
	var ps []C.Proxy
	for _, name := range list {
		p, ok := mapping[name]
		if !ok {
			return nil, fmt.Errorf("'%s' not found", name)
		}
		ps = append(ps, p)
	}
	return ps, nil
}

func getProviders(mapping map[string]types.ProxyProvider, list []string) ([]types.ProxyProvider, error) {
	var ps []types.ProxyProvider
	for _, name := range list {
		p, ok := mapping[name]
		if !ok {
			return nil, fmt.Errorf("'%s' not found", name)
		}

		if p.VehicleType() == types.Compatible {
			return nil, fmt.Errorf("proxy group %s can't contains in `use`", name)
		}
		ps = append(ps, p)
	}
	return ps, nil
}

func addTestUrlToProviders(providers []types.ProxyProvider, url string, expectedStatus utils.IntRanges[uint16], filter string, interval uint) {
	if len(providers) == 0 || len(url) == 0 {
		return
	}

	for _, pd := range providers {
		pd.RegisterHealthCheckTask(url, expectedStatus, filter, interval)
	}
}

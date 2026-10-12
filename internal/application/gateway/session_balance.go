package gateway

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strconv"

	appsec "github.com/zentrola/zentrola/internal/application/security"
)

// sessionRouteKey 隔离不同成员、虚拟 Key、协议和模型；原始客户端会话 ID 不进入 Redis 键。
func sessionRouteKey(identity appsec.PrincipalIdentity, request Request, model string) string {
	value, _ := json.Marshal([]any{
		identity.ID, identity.AccessKeyID, request.Protocol, model,
		request.ClientSessionID, request.ClientAgentID,
	})
	digest := sha256.Sum256(value)
	return hex.EncodeToString(digest[:])
}

func routeBalanceScore(key string, resourceID int64) [32]byte {
	return sha256.Sum256([]byte(key + ":" + strconv.FormatInt(resourceID, 10)))
}

func sameBalanceGroup(a, b Route) bool {
	return a.ProviderModelID == b.ProviderModelID &&
		a.EndpointProtocol == b.EndpointProtocol && a.AuthType == b.AuthType
}

func orderBalanceGroups(routes []Route, key string) {
	visited := make([]bool, len(routes))
	for start := range routes {
		if visited[start] {
			continue
		}
		indices := make([]int, 0)
		group := make([]Route, 0)
		for index := start; index < len(routes); index++ {
			if sameBalanceGroup(routes[start], routes[index]) {
				indices = append(indices, index)
				group = append(group, routes[index])
				visited[index] = true
			}
		}
		sort.SliceStable(group, func(i, j int) bool {
			left, right := routeBalanceScore(key, group[i].ResourceID), routeBalanceScore(key, group[j].ResourceID)
			for n := range left {
				if left[n] != right[n] {
					return left[n] > right[n]
				}
			}
			return group[i].ResourceID < group[j].ResourceID
		})
		for index, route := range group {
			routes[indices[index]] = route
		}
	}
}

func (s *Service) sessionRoutesForRequest(
	ctx context.Context, identity appsec.PrincipalIdentity, request Request, parsed Parsed, routes []Route,
) ([]Route, string, int64, error) {
	if request.ClientSessionID == "" || len(routes) == 0 {
		return routes, "", 0, nil
	}
	key := sessionRouteKey(identity, request, parsed.Model)
	ordered := append([]Route(nil), routes...)
	if parsed.PreviousResponseID == "" {
		orderBalanceGroups(ordered, key)
	}
	if s.sessionRoutes == nil {
		if parsed.PreviousResponseID != "" {
			return nil, "", 0, ErrUnavailable
		}
		return ordered, "", 0, nil
	}
	var assigned int64
	var err error
	if parsed.PreviousResponseID != "" {
		assigned, err = s.sessionRoutes.Get(ctx, key)
		if err != nil {
			return nil, "", 0, ErrUnavailable
		}
		if assigned == 0 {
			return nil, "", 0, ErrSessionRoute
		}
	} else {
		assigned, err = s.sessionRoutes.Assign(ctx, key, ordered[0].ResourceID)
	}
	if err != nil || assigned == 0 {
		return ordered, "", 0, nil
	}
	found := false
	for index := range ordered {
		if ordered[index].ResourceID == assigned {
			found = true
			selected := ordered[index]
			copy(ordered[1:index+1], ordered[:index])
			ordered[0] = selected
			break
		}
	}
	if !found && parsed.PreviousResponseID != "" {
		return nil, "", 0, ErrSessionRoute
	}
	return ordered, key, assigned, nil
}

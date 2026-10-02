package http

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	mgmt "github.com/zentrola/zentrola/internal/application/management"
	appsec "github.com/zentrola/zentrola/internal/application/security"
	"github.com/zentrola/zentrola/internal/domain/admin"
)

func routeID(r *http.Request, name string) (int64, error) { return positiveID(chi.URLParam(r, name)) }

func requestIDs(values []string) ([]int64, error) {
	ids := make([]int64, len(values))
	for index, value := range values {
		id, err := positiveID(value)
		if err != nil {
			return nil, err
		}
		ids[index] = id
	}
	return ids, nil
}

func optionalQueryValue(r *http.Request, name string) (string, error) {
	values, exists := r.URL.Query()[name]
	if !exists {
		return "", nil
	}
	if len(values) != 1 {
		return "", appsec.ErrInvalidArgument
	}
	return strings.TrimSpace(values[0]), nil
}

type statusUpdater func(context.Context, admin.Identity, int64, string, appsec.RequestMeta) error

func statusEndpoint(update statusUpdater) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		input, ok := decodeRequest[UpdateStatusRequest](w, r)
		if !ok {
			return
		}
		id, err := routeID(r, "id")
		if err != nil {
			securityError(w, r, err)
			return
		}
		err = update(r.Context(), adminFrom(r), id, input.Status, requestMeta(r))
		adminResult(w, r, http.StatusOK, UpdatedResponse{Updated: true}, err)
	}
}

type relationUpdater func(context.Context, admin.Identity, int64, int64, bool, appsec.RequestMeta) error

func relationEndpoint(update relationUpdater, childParam string, enabled bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := routeID(r, "id")
		if err != nil {
			securityError(w, r, err)
			return
		}
		childID, err := routeID(r, childParam)
		if err != nil {
			securityError(w, r, err)
			return
		}
		err = update(r.Context(), adminFrom(r), id, childID, enabled, requestMeta(r))
		adminResult(w, r, http.StatusOK, UpdatedResponse{Updated: true}, err)
	}
}

type detailLoader[T any] func(context.Context, admin.Identity, int64) (T, error)

func detailEndpoint[T any](load detailLoader[T]) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := routeID(r, "id")
		if err != nil {
			securityError(w, r, err)
			return
		}
		data, err := load(r.Context(), adminFrom(r), id)
		adminResult(w, r, http.StatusOK, data, err)
	}
}

type deleteAction func(context.Context, admin.Identity, int64, appsec.RequestMeta) error

func deleteEndpoint(remove deleteAction) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := routeID(r, "id")
		if err != nil {
			securityError(w, r, err)
			return
		}
		err = remove(r.Context(), adminFrom(r), id, requestMeta(r))
		adminResult(w, r, http.StatusOK, map[string]bool{"deleted": true}, err)
	}
}

func adminResult(w http.ResponseWriter, r *http.Request, status int, data any, err error) {
	if err != nil {
		securityError(w, r, err)
		return
	}
	writeJSON(w, r, status, response{Code: "OK", Data: data})
}

func listEndpoint[T any](load func(*http.Request, mgmt.Page) (mgmt.PageData[T], error), id func(T) int64, extraQueryNames ...string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p := mgmt.Page{Limit: 50}
		values := r.URL.Query()
		allowed := map[string]struct{}{"after": {}, "limit": {}}
		for _, name := range extraQueryNames {
			allowed[name] = struct{}{}
		}
		for name, list := range values {
			if _, ok := allowed[name]; !ok || len(list) != 1 {
				securityError(w, r, appsec.ErrInvalidArgument)
				return
			}
		}
		for _, name := range []string{"after", "limit"} {
			if list, ok := values[name]; ok {
				n, err := strconv.ParseInt(strings.TrimSpace(list[0]), 10, 64)
				if err != nil || n < 0 || (name == "limit" && (n < 1 || n > 100)) {
					securityError(w, r, appsec.ErrInvalidArgument)
					return
				}
				if name == "after" {
					p.After = n
				} else {
					p.Limit = int32(n)
				}
			}
		}
		p.ProbeNext = true
		page, err := load(r, p)
		if err != nil {
			securityError(w, r, err)
			return
		}
		data := page.Items
		var next *string
		if len(data) > int(p.Limit) {
			data = data[:p.Limit]
			value := strconv.FormatInt(id(data[len(data)-1]), 10)
			next = &value
		}
		adminResult(w, r, http.StatusOK, PageResponse[T]{Items: data, NextCursor: next, Total: page.Total}, nil)
	}
}

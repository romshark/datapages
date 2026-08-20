package app

import (
	"net/http"

	"github.com/romshark/datapages"
	"github.com/romshark/datapages/example/classifieds/app/domain"
)

// PageSearch is /search
type PageSearch struct {
	App *App
	Base
}

func (p PageSearch) GET(
	r *http.Request,
	session Session,
	query datapages.Query[SearchParams],
) (body datapages.Component, err error) {
	posts, err := p.App.repo.SearchPosts(r.Context(), domain.PostSearchParams{
		Term:     query.Values.Term,
		Category: query.Values.Category,
		PriceMin: query.Values.PriceMin,
		PriceMax: query.Values.PriceMax,
		Location: query.Values.Location,
	})
	if err != nil {
		return nil, err
	}

	baseData, err := p.baseData(r.Context(), session)
	if err != nil {
		return nil, err
	}

	categories, err := p.App.repo.MainCategories(r.Context())
	if err != nil {
		return nil, err
	}

	return pageSearch(session, query.Values, categories, posts, baseData), nil
}

// POSTParamChange is /search/paramchange/{$}
func (p PageSearch) POSTParamChange(
	r *http.Request,
	sse datapages.SSE,
	session Session,
	signals datapages.Signals[SearchParams],
) error {
	posts, err := p.App.repo.SearchPosts(sse.Context(), domain.PostSearchParams{
		Term:     signals.Values.Term,
		Category: signals.Values.Category,
		PriceMin: signals.Values.PriceMin,
		PriceMax: signals.Values.PriceMax,
		Location: signals.Values.Location,
	})
	if err != nil {
		return err
	}

	baseData, err := p.baseData(r.Context(), session)
	if err != nil {
		return err
	}

	categories, err := p.App.repo.MainCategories(r.Context())
	if err != nil {
		return err
	}

	ps := pageSearch(session, signals.Values, categories, posts, baseData)
	// Re-render the page (fat morph) and close stream.
	return sse.PatchElement(ps)
}

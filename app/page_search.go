package app

import (
	"datapages/app/domain"
	"net/http"

	"github.com/a-h/templ"
	"github.com/starfederation/datastar-go/datastar"
)

// PageSearch is /search
type PageSearch struct {
	App *App
	Base
}

func (p PageSearch) GET(
	r *http.Request,
	session SessionJWT,
	query SearchParams,
) (body templ.Component, err error) {
	posts, err := p.App.repo.SearchPosts(r.Context(), domain.PostSearchParams{
		Term:     query.Term,
		Category: query.Category,
		PriceMin: query.PriceMin,
		PriceMax: query.PriceMax,
		Location: query.Location,
	})
	if err != nil {
		return nil, err
	}

	baseData, err := p.baseData(r.Context(), session)
	if err != nil {
		return nil, err
	}

	return pageSearch(session, query, posts, baseData), nil
}

// POSTParamChange is /search/paramchange/{$}
func (p PageSearch) POSTParamChange(
	r *http.Request,
	sse *datastar.ServerSentEventGenerator,
	session SessionJWT,
	signals SearchParams,
) error {
	posts, err := p.App.repo.SearchPosts(sse.Context(), domain.PostSearchParams{
		Term:     signals.Term,
		Category: signals.Category,
		PriceMin: signals.PriceMin,
		PriceMax: signals.PriceMax,
		Location: signals.Location,
	})
	if err != nil {
		return err
	}

	baseData, err := p.baseData(r.Context(), session)
	if err != nil {
		return err
	}

	ps := pageSearch(session, signals, posts, baseData)
	// Re-render the page (fat morph) and close stream.
	return sse.PatchElementTempl(ps)
}

package finance

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/sakusi4/monolith/internal/money"
	"github.com/sakusi4/monolith/web"
)

const assetsURL = "/finance/assets"

type handler struct {
	store *Store
}

type assetForm struct {
	Title  string
	Types  []AssetType
	Type   AssetType
	Name   string
	Amount string
	Error  string
}

func NewHandler(store *Store) http.Handler {
	h := &handler{store: store}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /finance/assets", h.listAssets)
	mux.HandleFunc("GET /finance/assets/new", h.newAsset)
	mux.HandleFunc("POST /finance/assets/new", h.createAsset)
	mux.HandleFunc("GET /finance/assets/{id}/edit", h.editAsset)
	mux.HandleFunc("POST /finance/assets/{id}/edit", h.updateAsset)
	mux.HandleFunc("POST /finance/assets/{id}/delete", h.deleteAsset)
	return mux
}

func (h *handler) listAssets(w http.ResponseWriter, r *http.Request) {
	assets, err := h.store.Assets(r.Context())
	if err != nil {
		web.ServerError(w, r, err)
		return
	}
	web.Render(w, r, http.StatusOK, "asset_list", assets)
}

func (h *handler) newAsset(w http.ResponseWriter, r *http.Request) {
	web.Render(w, r, http.StatusOK, "asset_form", assetForm{Title: "Add asset", Types: assetTypes})
}

func (h *handler) createAsset(w http.ResponseWriter, r *http.Request) {
	form, in, ok := readAssetForm(r, "Add asset")
	if !ok {
		web.Render(w, r, http.StatusUnprocessableEntity, "asset_form", form)
		return
	}
	_, err := h.store.CreateAsset(r.Context(), in)
	respondSave(w, r, form, err)
}

func (h *handler) editAsset(w http.ResponseWriter, r *http.Request) {
	id, ok := assetID(r)
	if !ok {
		http.NotFound(w, r)
		return
	}
	a, err := h.store.Asset(r.Context(), id)
	if errors.Is(err, ErrAssetNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		web.ServerError(w, r, err)
		return
	}
	web.Render(w, r, http.StatusOK, "asset_form", assetForm{
		Title:  "Edit asset",
		Types:  assetTypes,
		Type:   a.Type,
		Name:   a.Name,
		Amount: money.InputUSD(a.AmountCents),
	})
}

func (h *handler) updateAsset(w http.ResponseWriter, r *http.Request) {
	id, ok := assetID(r)
	if !ok {
		http.NotFound(w, r)
		return
	}
	form, in, ok := readAssetForm(r, "Edit asset")
	if !ok {
		web.Render(w, r, http.StatusUnprocessableEntity, "asset_form", form)
		return
	}
	_, err := h.store.UpdateAsset(r.Context(), id, in)
	respondSave(w, r, form, err)
}

func (h *handler) deleteAsset(w http.ResponseWriter, r *http.Request) {
	id, ok := assetID(r)
	if !ok {
		http.NotFound(w, r)
		return
	}
	err := h.store.DeleteAsset(r.Context(), id)
	if errors.Is(err, ErrAssetNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		web.ServerError(w, r, err)
		return
	}
	http.Redirect(w, r, assetsURL, http.StatusSeeOther)
}

func readAssetForm(r *http.Request, title string) (assetForm, AssetInput, bool) {
	form := assetForm{
		Title:  title,
		Types:  assetTypes,
		Type:   AssetType(r.PostFormValue("type")),
		Name:   r.PostFormValue("name"),
		Amount: r.PostFormValue("amount"),
	}
	cents, ok := money.ParseUSD(form.Amount)
	if !ok {
		form.Error = "Enter the amount in dollars, like 1,234.56."
		return form, AssetInput{}, false
	}
	return form, AssetInput{Type: form.Type, Name: form.Name, AmountCents: cents}, true
}

func respondSave(w http.ResponseWriter, r *http.Request, form assetForm, err error) {
	switch {
	case errors.Is(err, ErrInvalidAsset):
		form.Error = "Check the fields and try again."
		web.Render(w, r, http.StatusUnprocessableEntity, "asset_form", form)
	case errors.Is(err, ErrAssetNotFound):
		http.NotFound(w, r)
	case err != nil:
		web.ServerError(w, r, err)
	default:
		http.Redirect(w, r, assetsURL, http.StatusSeeOther)
	}
}

func assetID(r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	return id, err == nil
}

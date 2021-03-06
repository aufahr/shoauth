package shoauth

import (
	"net/http"
	"strings"
)

type shopifyOauthHandler struct {
	successHandler http.Handler
	failureHandler http.Handler
	config         *ShopifyConfig
	ShopifyPersistence
}

// NewShopifyOauthHandler returns the middleware handler that handles Shopify
// oauth requests and responses.  It will call successHandler.ServeHTTP on a
// successful installation or verification, and will call
// failureHandler.ServeHTTP on an unsuccessful installation or verification.
// The user must pass a shopifyPersistence-satisfying struct and any functions
// they wish to operate on the default config object.
func NewShopifyOauthHandler(successHandler http.Handler, failureHandler http.Handler, persistence ShopifyPersistence, configOptions ...func(*ShopifyConfig)) *shopifyOauthHandler {
	// Set some sensible defaults.
	config := &ShopifyConfig{
		RedirectURI: "",
		Webhooks:    make(map[string]string),
		Scripts:     make(map[string][]string),
	}

	// Apply the custom config functions passed.
	for _, f := range configOptions {
		f(config)
	}

	return &shopifyOauthHandler{
		successHandler:     successHandler,
		failureHandler:     failureHandler,
		ShopifyPersistence: persistence,
		config:             config,
	}
}

func (s *shopifyOauthHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {

	if len(r.FormValue("shop")) == 0 {
		s.failureHandler.ServeHTTP(w, r)
		return
	}

	// If this shop has not installed our app, and we do not have a code
	// parameter, redirect them to the install page
	if !s.InstallationExists(r.FormValue("shop")) && len(r.FormValue("code")) == 0 {
		// Construct our scopes parameter
		scopeParameter := ""
		if len(s.config.Scopes) > 0 {
			scopeParameter = "&scope=" + strings.Join(s.config.Scopes, ",")
		}
		redirectURL := "https://" + r.FormValue("shop") + "/admin/oauth/authorize?client_id=" + s.config.ClientID + scopeParameter
		if len(s.config.RedirectURI) > 0 {
			redirectURL += "&redirect_uri=" + s.config.RedirectURI
		}
		http.Redirect(w, r, redirectURL, http.StatusMovedPermanently)
		return
	}

	// If this shop has not installed our app, and we do have a code parameter,
	// attempt an installation.
	if !s.InstallationExists(r.FormValue("shop")) {
		// We perform the installation - if it fails, call the app's
		// failure handler.  Otherwise, we open up the app.  If it's embedded,
		// we do this within the admin interface.  Otherwise, just call the app
		// handler.
		if err := s.performInstallation(r.FormValue("shop"), r.FormValue("code")); err != nil {
			s.failureHandler.ServeHTTP(w, r)
		} else {
			if s.config.IsEmbedded {
				http.Redirect(w, r, "https://"+r.FormValue("shop")+"/admin/apps/"+s.config.ClientID, http.StatusMovedPermanently)
			} else {
				s.successHandler.ServeHTTP(w, r)
			}
		}
		// If this is not an installation request, we must validate that it has
		// actually come from shopify according to their predefined rules.
	} else {
		if err := validateRequest(r, s.config.SharedSecret); err != nil {
			s.failureHandler.ServeHTTP(w, r)
		} else {
			s.successHandler.ServeHTTP(w, r)
		}
	}
}

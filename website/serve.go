// +build ignore

// Copyright 2017 EcoSystem Software LLP

// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at

// 	http://www.apache.org/licenses/LICENSE-2.0

// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

//Use the forked version of the go-jwt-middlware, not the auth0 version

package website

import (
	"html/template"
	"net/http"
	"time"

	jwt "github.com/dgrijalva/jwt-go"
	"github.com/goware/cors"
	jwtmiddleware "github.com/jonbonazza/go-jwt-middleware"
	"github.com/spf13/afero"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	gin "gopkg.in/gin-gonic/gin.v1"

	"fmt"

	"log"

	"path"

	"github.com/ecosystemsoftware/ecosystem/ecosql"
	"github.com/ecosystemsoftware/ecosystem/handlers"
	"github.com/ecosystemsoftware/ecosystem/handlers/admin"
	"github.com/ecosystemsoftware/ecosystem/handlers/api"
	"github.com/ecosystemsoftware/ecosystem/handlers/web"
	"github.com/ecosystemsoftware/ecosystem/templates"
	eco "github.com/ecosystemsoftware/ecosystem/utilities"
	"github.com/pressly/chi"
	"github.com/pressly/chi/middleware"
)

var nowebsite, noadminpanel bool
var smtpPW string

func init() {
	RootCmd.AddCommand(serveCmd)

	serveCmd.Flags().BoolVarP(&nowebsite, "nowebsite", "w", false, "Disable website/HTML server")
	serveCmd.Flags().BoolVarP(&noadminpanel, "noadminpanel", "a", false, "Disable admin panel server")

	serveCmd.Flags().String("smtppw", "", "SMTP server password for outgoing mail")
	viper.BindPFlag("smtppw", serveCmd.Flags().Lookup("smtppw"))

	serveCmd.Flags().BoolP("demomode", "d", false, "Run server in demo mode")
	viper.BindPFlag("demomode", serveCmd.Flags().Lookup("demomode"))

	serveCmd.Flags().StringP("secret", "s", "", "Secure secret for signing JWT")
	viper.BindPFlag("secret", serveCmd.Flags().Lookup("secret"))

}

// serveCmd represents the serve command
var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Starts the EcoSystem server",
	Long: `Start EcoSystem with all 3 servers: api, web and admin panel.
	Use flags to disable web and admin panel serving if you plan to host them elsewhere or ony use the API server`,
	RunE: serve,
}

func serve(cmd *cobra.Command, args []string) error {

	preServe()

	if !noadminpanel {
		serveAdminPanel()
	}
	if !nowebsite {
		serveWebsite()
	}
	serveAPI()

	return nil
}

func preServe() {

	//Check to make sure a secret has been provided
	//No default provided as a security measure, server will exit of nothing provided
	if viper.GetString("secret") == "" {
		log.Fatal("No signing secret provided")
	}

	//Set up the email server and test
	err := eco.EmailSetup()
	if err != nil {
		log.Println("Error setting up email system: ", err.Error())
		log.Println("Email system will not function")
	}

	//Establish a temporary connection as the super user
	dbTemp := eco.SuperUserDBConfig.ReturnDBConnection("")

	//Generate a random server password, set it and get out
	serverPW := eco.RandomString(16)
	_, err = dbTemp.Exec(fmt.Sprintf(ecosql.ToSetServerRolePassword, serverPW))
	if err != nil {
		log.Fatal("Error setting server role password: ", err.Error())
	}

	dbTemp.Close()

	//Establish a permanent connection
	eco.DB = eco.ServerUserDBConfig.ReturnDBConnection(serverPW)

}

func serveAPI() {

	// Basic CORS
	// for more ideas, see: https://developer.github.com/v3/#cross-origin-resource-sharing
	cors := cors.New(cors.Options{
		// AllowedOrigins: []string{"https://foo.com"}, // Use this to allow specific origin hosts
		AllowedOrigins:   []string{"*"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS", "PATCH", "SEARCH"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-CSRF-Token"},
		ExposedHeaders:   []string{"Link"},
		AllowCredentials: true,
		MaxAge:           300, // Maximum value not ignored by any of major browsers
	})

	jwtMiddleware := jwtmiddleware.New(jwtmiddleware.Options{
		ValidationKeyGetter: func(token *jwt.Token) (interface{}, error) {
			return []byte(viper.GetString("secret")), nil
		},
		// When set, the middleware verifies that tokens are signed with the specific signing algorithm
		// If the signing method is not constant the ValidationKeyGetter callback can be used to implement additional checks
		// Important to avoid security issues described here: https://auth0.com/blog/2015/03/31/critical-vulnerabilities-in-json-web-token-libraries/
		SigningMethod: jwt.SigningMethodHS256,
	})

	r := chi.NewRouter()

	// A good base middleware stack
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(cors.Handler) //Activate CORS middleware

	// When a client closes their connection midway through a request, the
	// http.CloseNotifier will cancel the request context (ctx).
	r.Use(middleware.CloseNotify)

	// Set a timeout value on the request context (ctx), that will signal
	// through ctx.Done() that the request has timed out and further
	// processing should be stopped.
	r.Use(middleware.Timeout(60 * time.Second))

	//Main Routing Tree
	//The base slug is 'schema'
	r.Route("/:schema", func(r chi.Router) {
		//Activate JWT middleware right at the base
		r.Use(jwtMiddleware.Handler)
		//If a valid JWT is present, a user id and role will be assigned
		r.Use(handlers.Authorizator)
		//Next slug is 'table'
		r.Route("/:table", func(r chi.Router) {
			//Use middleware to add the schema, table and queries to the context
			r.Use(handlers.AddSchemaAndTableToContext)
			r.Get("/", api.ShowList)      // GET /schema/table
			r.Post("/", api.InsertRecord) // PUT /schema/table
			//Final level is 'record'
			r.Route("/:record", func(r chi.Router) {
				//Use middleware to add the record to the context
				r.Use(handlers.AddRecordToContext)
				r.Get("/", api.ShowSingle)      // GET /schema/table/record
				r.Patch("/", api.UpdateRecord)  // PATCH /schema/table/record
				r.Delete("/", api.DeleteRecord) // DELETE /schema/table/record

			})

		})
	})

	//Resized image route
	//Note format: /images/[IMAGE NAME WITH OPTIONAL PATH]?width=[WIDTH IN PIXELS]
	//TODO: this will serve image directories from bundles whether they are installed or not
	r.Route("/images", func(r chi.Router) {
		r.With(handlers.AddImageDetails).Get("/*", handlers.ShowImage)
	})

	r.Get("/newuser", handlers.RequestNewUserToken)
	r.Post("/login", handlers.RequestLogin)
	r.Post("/magiccode", handlers.MagicCode)

	http.ListenAndServe(":"+viper.GetString("apiPort"), r)

}

func serveWebsite() {

	jwtMiddleware := jwtmiddleware.New(jwtmiddleware.Options{
		ValidationKeyGetter: func(token *jwt.Token) (interface{}, error) {
			return []byte(viper.GetString("secret")), nil
		},
		// When set, the middleware verifies that tokens are signed with the specific signing algorithm
		// If the signing method is not constant the ValidationKeyGetter callback can be used to implement additional checks
		// Important to avoid security issues described here: https://auth0.com/blog/2015/03/31/critical-vulnerabilities-in-json-web-token-libraries/
		SigningMethod: jwt.SigningMethodHS256,
	})

	r := chi.NewRouter()

	// A good base middleware stack
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(cors.Handler) //Activate CORS middleware

	// When a client closes their connection midway through a request, the
	// http.CloseNotifier will cancel the request context (ctx).
	r.Use(middleware.CloseNotify)

	// Set a timeout value on the request context (ctx), that will signal
	// through ctx.Done() that the request has timed out and further
	// processing should be stopped.
	r.Use(middleware.Timeout(60 * time.Second))

	//Templates
	//Start with the ecosystem.js templates
	html := template.Must(template.New("ecosystem.js").Parse(templates.EcoSystemJS))
	//Add all the bundles templates
	html, err := html.ParseGlob("bundles/**/templates/**/*.html")
	//TODO: this will serve templates from bundles whether they are installed or not. Need to fix
	if err != nil {
		log.Println("Template error:", err.Error())
	} else {
		//Set the templates on the server
		 .SetHTMLTemplate(html)
	}

	//Ecosystem JS
	webServer.GET("/ecosystem.js", web.GetEcoSystemJS)

	//Resized image route
	//Note format: /images/[IMAGE NAME WITH OPTIONAL PATH]?width=[WIDTH IN PIXELS]
	//TODO: this will serve image directories from bundles whether they are installed or not
	//Use star instead fo colons to allow for paths
	// r.Route("/images", func(r chi.Router) {
	// 	r.Use(handlers.AddImageDetails)
	// 	r.Get("/*image", handlers.ShowImage)
	// })

	//Homepage and web categories
	webServer.GET("/", web.WebShowEntryPage)
	webServer.GET("/"+viper.GetString("publicSiteSlug"), web.WebShowEntryPage)
	webServer.GET("category/:schema/:table/:cat", web.WebShowCategory)

	//Bundle public directories
	public := webServer.Group("/public")
	{
		//For each bundle installed - add that bundle's public directory contents at TOPLEVEL/public/BUNDLENAME
		bundles := viper.GetStringSlice("bundlesInstalled")

		for _, v := range bundles {
			public.StaticFS(v, http.Dir(path.Join("bundles", v, "public")))

		}

	}

	//Unprotected HTML routes.  Authentiaction middleware is not activated
	//so there is no need for the browser to present a JWT
	//Database will always be queried with role 'web'.  Therefore give priveleges to this role
	//to all tables that are intended to be public
	//This is intended for the main site pages that are public and available to crawlers
	site := webServer.Group(viper.GetString("publicSiteSlug"))

	{
		site.GET(":schema", web.WebShowEntryPage)
		site.GET(":schema/:table", web.WebShowList)
		site.GET(":schema/:table/:slug", web.WebShowSingle)
	}

	//Protected HTML routes.
	//Authentication middlware is actiaved so a JWT must be presented by the browser
	// These are used as partials when you want to
	//return formatted HTML specified to the logged in user (e.g. a cart)
	private := webServer.Group(viper.GetString("privateSiteSlug"))

	{
		//private.Use(handlers.AuthMiddleware.MiddlewareFunc())
		private.GET(":schema", web.WebShowEntryPage)
		private.GET(":schema/:table", web.WebShowList)
		private.GET(":schema/:table/:slug", web.WebShowSingle)
	}

	go webServer.Run(":" + viper.GetString("websitePort"))

}

func serveAdminPanel() {

	adminServer := gin.Default()

	//Group views from bundles
	views := adminServer.Group("/views")
	{
		//views.Use(handlers.MakeJSON)                   //Activate JSON Header middleware
		views.GET("", admin.AdminShowConcatenatedJSON) //Concatenates view.json from each bundle
	}

	//Group menus from bundles
	menu := adminServer.Group("/menu")
	{
		//menu.Use(handlers.MakeJSON)                   //Activate JSON Header middleware
		menu.GET("", admin.AdminShowConcatenatedJSON) //Concatenates menu.json from each bundle
	}

	//Serve the Polymer app at /admin
	// Simple way - just map the /admin to the serving directory
	// Downside is that you can only enter the app at one place
	//adminServer.StaticFS("/admin", http.Dir(viper.GetString("adminPanelServeDirectory")+"/"))

	//Hard way:
	//Router seems to have a hard time with widlcard conflicts, so this is the only way
	//Ive found to do it
	//(at the moment) all valid views are /admin/view - so in all those cases serve the index.html
	adminServer.GET("/admin/view/*anything", func(c *gin.Context) {
		c.File("./" + viper.GetString("adminPanelServeDirectory") + "/index.html")
	})

	//Serve the admin imports dynamically generated html
	html := template.Must(template.New("admin-imports.html").Parse(templates.Admin))
	adminServer.SetHTMLTemplate(html)
	adminServer.GET("admin/imports.html", admin.AdminGetImports)

	// //Otherwise
	// //Serve these static files
	adminServer.StaticFile("admin", viper.GetString("adminPanelServeDirectory")+"/index.html")
	adminServer.StaticFile("admin/", viper.GetString("adminPanelServeDirectory")+"/index.html")
	adminServer.StaticFile("admin/index.html", viper.GetString("adminPanelServeDirectory")+"/index.html")
	adminServer.StaticFile("admin/manifest.json", viper.GetString("adminPanelServeDirectory")+"/manifest.json")
	adminServer.StaticFile("admin/service-worker.js", viper.GetString("adminPanelServeDirectory")+"/service-worker.js")
	adminServer.StaticFile("admin/sw-precache-config.js", viper.GetString("adminPanelServeDirectory")+"/sw-precache-config.js")

	// //And serve these subdirectories as file systems
	adminServer.StaticFS("/admin/bower_components", http.Dir(viper.GetString("adminPanelServeDirectory")+"/bower_components"))
	adminServer.StaticFS("/admin/src", http.Dir(viper.GetString("adminPanelServeDirectory")+"/src"))
	adminServer.StaticFS("/admin/images", http.Dir(viper.GetString("adminPanelServeDirectory")+"/images"))

	//Serve bundle customisation files at /bundles/[BUNDLENAME]
	custom := adminServer.Group("/bundles")

	//For each bundle present - add that bundle's admin directory contents at TOPLEVEL/custom/BUNDLENAME
	if bundleDirectoryContents, err := afero.ReadDir(eco.AppFs, "bundles"); err == nil {
		for _, v := range bundleDirectoryContents {
			if v.IsDir() {
				custom.StaticFS(v.Name(), http.Dir(path.Join("bundles", v.Name(), "admin-panel")))
			}
		}
	}

	go adminServer.Run(":" + viper.GetString("adminPanelPort"))

}

// 	api.Handle("SEARCH", "/:schema/:table/", handlers.ReturnBlank) //Useful for when blank searches are sent by client, to avoid errors
// 	api.Handle("SEARCH", "/:schema/:table/:searchTerm", handlers.SearchList)

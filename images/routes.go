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

package images

import (
	"github.com/ecosystemsoftware/ecosystem/core"
	"github.com/pressly/chi"
)

func setRoutes() {

	//Note format: /images/[IMAGE NAME WITH OPTIONAL PATH]?width=[WIDTH IN PIXELS]
	//TODO: this will serve image directories from bundles whether they are installed or not
	core.Router.Route("/images", func(r chi.Router) {
		r.With(AddImageDetails).Get("/*", ShowImage)
	})

}

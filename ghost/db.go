// Copyright 2017 Jonathan Pincas

// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at

// 	http://www.apache.org/licenses/LICENSE-2.0

// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package ghost

import (
	"database/sql"

	_ "github.com/lib/pq"
	"github.com/spf13/viper"
)

//dbConfig holds all the necessary information for a datbase connection
type dbConfig struct {
	user, pw, server, port, dbName string
	disableSSL                     bool
}

//SuperUserDBConfig is the connection configuration for the super user
//ServerUserDBConfig is the connection configuration for the 'server' role user
var (
	SuperUserDBConfig  dbConfig
	ServerUserDBConfig dbConfig
)

//TestDBConfig is a ready to go database config for testing purposes
//TODO: at the moment the config for testing is fixed - need to work out
//clean way of allowing users to specify different config
var TestDBConfig = dbConfig{
	user:       "postgres",
	server:     "localhost",
	port:       "5432",
	dbName:     "testing",
	disableSSL: true,
}

func (d *dbConfig) SetupConnection(isSuperUser bool) {

	//Default configuration
	d.user = "server"
	d.server = App.Config.PgServer
	d.port = App.Config.PgPort
	d.dbName = App.Config.PgDBName
	d.disableSSL = App.Config.PgDisableSSL

	//For super user
	if isSuperUser {
		d.user = viper.GetString("pgSuperUser")
		d.pw = viper.GetString("pgpw")
	}

}

//ReturnDBConnection returns a App.DB connection pool using the connection parameters in a dbConfig struct
//and an optional server password which can be passed in
func (d dbConfig) ReturnDBConnection(serverPW string) *sql.DB {

	dbConnectionString := d.getDBConnectionString(serverPW)
	return connectToDB(dbConnectionString)

}

//getDBConnectionString returns a correctly formated Postgres connection string from
//the config struct.  If there is no pw in the struct (as is the case for )
func (d dbConfig) getDBConnectionString(serverPW string) (dbConnectionString string) {

	//If this is a connection for a server role, use the password supplied as a parameter
	//Otherwise ignore that parameter
	if d.user == "server" {
		d.pw = serverPW
	}

	//Set the password string if a password has been supplied
	//If not leave it blank - this stops any errors for blank passwords
	pwString := ""
	if d.pw != "" {
		pwString = ":" + d.pw
	}
	dbConnectionString = "postgres://" + d.user + pwString + "@" + d.server + ":" + d.port + "/" + d.dbName
	//If disabled SSL flag specified,
	if d.disableSSL {
		dbConnectionString += "?sslmode=disable"
	}
	return
}

//connectToDB connects to the database and returns a connection pool
func connectToDB(dbConnectionString string) *sql.DB {
	//Initialise database
	Log("DB", true, "Connecting to "+dbConnectionString, nil)

	dbConnection, _ := sql.Open("postgres", dbConnectionString)
	//Ping database to check connectivity
	if err := dbConnection.Ping(); err != nil {
		LogFatal("DB", false, "Error connecting to Postgres as super user during setup. ", err)
	} else {
		Log("DB", true, "Connected to "+dbConnectionString, nil)
	}
	return dbConnection
}

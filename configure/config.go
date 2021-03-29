package configure

import (
	"bytes"
	"encoding/json"
	"strings"

	nested "github.com/antonfisher/nested-logrus-formatter"
	"github.com/kr/pretty"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

type ServerCfg struct {
	Level      string `mapstructure:"level"`
	ConfigFile string `mapstructure:"config_file"`
	RedisURI   string `mapstructure:"redis_uri"`
	MongoURI   string `mapstructure:"mongo_uri"`
	MongoDB    string `mapstructure:"mongo_db"`
	ExitCode   int    `mapstructure:"exit_code"`
}

// default config
var defaultConf = ServerCfg{
	ConfigFile: "config.yaml",
}

var Config = viper.New()

func initLog() {
	if l, err := log.ParseLevel(Config.GetString("level")); err == nil {
		log.SetLevel(l)
		// log.SetReportCaller(l == log.DebugLevel)
	}
	log.SetFormatter(&nested.Formatter{
		HideKeys:    true,
		FieldsOrder: []string{"component", "category"},
	})
}

func checkErr(err error) {
	if err != nil {
		panic(err)
	}
}

func init() {
	// Default config
	b, _ := json.Marshal(defaultConf)
	defaultConfig := bytes.NewReader(b)
	viper.SetConfigType("json")
	checkErr(viper.ReadConfig(defaultConfig))
	checkErr(Config.MergeConfigMap(viper.AllSettings()))

	// Flags
	pflag.String("config_file", "config.yaml", "configure filename")
	pflag.String("level", "info", "Log level")
	pflag.String("redis_uri", "", "Address for the redis server.")
	pflag.String("mongo_uri", "", "Address for the mongodb server.")
	pflag.String("mongodb", "", "Database for the mongodb connection.")
	pflag.String("version", "1.0", "Version of the system.")
	pflag.Int("exit_code", 0, "Status code for successful and graceful shutdown, [0-125].")
	pflag.Parse()
	checkErr(Config.BindPFlags(pflag.CommandLine))

	// File
	Config.SetConfigFile(Config.GetString("config_file"))
	Config.AddConfigPath(".")
	err := Config.ReadInConfig()
	if err != nil {
		log.Warning(err)
		log.Info("Using default config")
	} else {
		checkErr(Config.MergeInConfig())
	}

	// Environment
	replacer := strings.NewReplacer(".", "_")
	Config.SetEnvKeyReplacer(replacer)
	Config.AllowEmptyEnv(true)
	Config.AutomaticEnv()

	// Log
	initLog()

	// Print final config
	c := ServerCfg{}
	checkErr(Config.Unmarshal(&c))
	log.Debugf("Current configurations: \n%# v", pretty.Formatter(c))
}

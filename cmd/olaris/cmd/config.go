package cmd

// Config is the base struct populated from the configuration
// file on disk by viper
type Config struct {
	Debug   DebugConfig
	Server  ServerConfig
	Library LibraryConfig
}

// DebugConfig is for debug settings
type DebugConfig struct {
	StreamingPages bool
	TranscoderLog  bool
}

// ServerConfig is for server settings
type ServerConfig struct {
	Port             int
	Verbose          bool
	DBLog            bool
	DirectFileAccess bool
	SystemFFMPEG     bool
}

// LibraryConfig is for library settings
type LibraryConfig struct {
	// To be continued
}

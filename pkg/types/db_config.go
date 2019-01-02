package types

// DBConfig is the DB configuration.
type DBConfig struct {
	Host     string `toml:"host" json:"host"`
	User     string `toml:"user" json:"user"`
	Password string `toml:"password" json:"password"`
	Port     int    `toml:"port" json:"port"`

	Timeout string `toml:"timeout" json:"timeout"`

	ReplicationUser     string `toml:"replication_user" json:"replication_user"`
	ReplicationPassword string `toml:"replication_password" json:"replication_password"`
	ReplicationNet      string `toml:"replication_net" json:"replication_net"`
}

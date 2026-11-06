package consul

import "context"

// ConfigPrefix es el prefijo KV del que la API lee su configuración.
const ConfigPrefix = "pdf-extractext/config/api/"

// KVSource abstrae la lectura de KV para poder testear sin Consul.
type KVSource interface {
	KVList(ctx context.Context, prefix string) (map[string]string, error)
}

// LoadConfigKV devuelve las claves relativas del prefijo de configuración.
func LoadConfigKV(ctx context.Context, source KVSource) (map[string]string, error) {
	return source.KVList(ctx, ConfigPrefix)
}

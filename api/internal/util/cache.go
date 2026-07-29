package util

import (
	"strings"
	"sync"
)

// promptCache in-memory кэш для промтов
type promptCache struct {
	cache sync.Map
}

var globalCache = &promptCache{}

// Get получает значение из кэша
func (c *promptCache) Get(key string) (string, bool) {
	v, ok := c.cache.Load(key)
	if !ok {
		return "", false
	}
	return v.(string), true
}

// Set сохраняет значение в кэш
func (c *promptCache) Set(key, value string) {
	c.cache.Store(key, value)
}

// Invalidate удаляет значение из кэша
func (c *promptCache) Invalidate(key string) {
	c.cache.Delete(key)
}

// InvalidateAll очищает весь кэш
func (c *promptCache) InvalidateAll() {
	c.cache = sync.Map{}
}

// cachedLoadPrompt загружает промпт с кэшированием (без поддиректорий)
func cachedLoadPrompt(name, tp, provider, version string) (string, error) {
	key := version + ":" + provider + ":" + name + ":" + tp

	if cached, ok := globalCache.Get(key); ok {
		return cached, nil
	}

	result, err := loadPrompt(name, tp, provider, version)
	if err != nil {
		return "", err
	}

	globalCache.Set(key, result)
	return result, nil
}

// cachedLoadPromptSubdirs загружает промпт с кэшированием и поддиректориями
func cachedLoadPromptSubdirs(name, tp, provider, version string, subdirs ...string) (string, error) {
	key := version + ":" + provider + ":" + name + ":" + tp + ":" + strings.Join(subdirs, "/")

	if cached, ok := globalCache.Get(key); ok {
		return cached, nil
	}

	result, err := loadPrompt(name, tp, provider, version, subdirs...)
	if err != nil {
		return "", err
	}

	globalCache.Set(key, result)
	return result, nil
}

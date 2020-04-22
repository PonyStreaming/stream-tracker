package main

import (
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	"gopkg.in/yaml.v2"
)

type StreamMapper struct {
	configPath     string
	mapping        map[string]string
	reverseMapping map[string]string
	outputs        map[string]string
	mutex          sync.RWMutex
	lastModified   time.Time
}

type StreamMappingConfig struct {
	Mapping map[string]string `yaml:"mapping"`
	Outputs map[string]string `yaml:"outputs"`
}

func NewStreamMapper(configPath string) (*StreamMapper, error) {
	sm := new(StreamMapper)
	sm.configPath = configPath
	return sm, sm.parse()
}

func (sm *StreamMapper) Run() {
	go func() {
		for {
			time.Sleep(10 * time.Second)
			info, err := os.Stat(sm.configPath)
			if err != nil {
				log.Printf("Couldn't stat %q: %v\n", sm.configPath, err)
				continue
			}
			sm.mutex.RLock()
			mtime := sm.lastModified
			sm.mutex.RUnlock()
			if info.ModTime().Equal(mtime) {
				continue
			}
			sm.mutex.Lock()
			sm.lastModified = info.ModTime()
			sm.mutex.Unlock()
			if err := sm.parse(); err != nil {
				log.Printf("Couldn't parse %q: %v\n", sm.configPath, err)
				continue
			}
			log.Println("Updated stream mapping")
		}
	}()
}

func (sm *StreamMapper) HasStreamKey(key string) bool {
	sm.mutex.RLock()
	defer sm.mutex.RUnlock()
	_, ok := sm.reverseMapping[key]
	return ok
}

func (sm *StreamMapper) StreamNameFromKey(key string) string {
	sm.mutex.RLock()
	defer sm.mutex.RUnlock()
	return sm.reverseMapping[key]
}

func (sm *StreamMapper) GetStreams() map[string]string {
	sm.mutex.RLock()
	defer sm.mutex.RUnlock()
	ret := map[string]string{}
	for k, v := range sm.mapping {
		ret[k] = v
	}
	return ret
}

func (sm *StreamMapper) GetOutputs() map[string]string {
	sm.mutex.RLock()
	defer sm.mutex.RUnlock()
	ret := map[string]string{}
	for k, v := range sm.outputs {
		ret[k] = v
	}
	return ret
}

func (sm *StreamMapper) parse() error {
	f, err := os.Open(sm.configPath)
	if err != nil {
		return fmt.Errorf("failed to open %q: %v", sm.configPath, err)
	}
	defer f.Close()
	smc := StreamMappingConfig{}
	if err := yaml.NewDecoder(f).Decode(&smc); err != nil {
		return fmt.Errorf("failed to parse yaml: %v", err)
	}
	reverseMapping := map[string]string{}
	for k, v := range smc.Mapping {
		if otherKey, ok := reverseMapping[v]; ok {
			return fmt.Errorf("duplicate stream key: %q (for %q and %q)", v, k, otherKey)
		}
		reverseMapping[v] = k
	}
	sm.mutex.Lock()
	sm.mapping = smc.Mapping
	sm.reverseMapping = reverseMapping
	sm.outputs = smc.Outputs
	sm.mutex.Unlock()
	return nil
}

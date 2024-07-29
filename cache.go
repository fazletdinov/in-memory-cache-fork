package inmemorycache

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"
	"unsafe"
)

func New(defaultExpiration, cleanupInterval, cronSavingDataToCacheDelay time.Duration) *InMemoryCache {

	items := make(map[string]CacheItem)

	cache := InMemoryCache{
		RWMutex:           sync.RWMutex{},
		defaultExpiration: defaultExpiration,
		cleanupInterval:   cleanupInterval,
		items:             items,
	}

	if cleanupInterval > 0 {
		go cache.gC()
	}

	if cronSavingDataToCacheDelay > 0 {
		go cache.cronSavingDataToFile(cronSavingDataToCacheDelay)
	}

	return &cache
}

func (c *InMemoryCache) Set(key string, value interface{}, duration time.Duration) {
	var expiration int64

	if duration > 0 {
		expiration = time.Now().Add(duration).UnixNano()
	}

	if duration < 0 {
		expiration = int64(NoExpiration)
	}

	if duration == DefaultExpiration {
		expiration = time.Now().Add(c.defaultExpiration).UnixNano()
	}

	c.Lock()

	defer c.Unlock()

	c.items[key] = CacheItem{
		Key:        key,
		Value:      value,
		Created:    time.Now(),
		Expiration: expiration,
	}
}

func (c *InMemoryCache) Get(key string) (*CacheItem, error) {
	c.RLock()

	defer c.RUnlock()

	item, found := c.items[key]

	if !found {
		return nil, fmt.Errorf("item with key %s does not exist", key)
	}

	if item.Expiration > 0 {
		if time.Now().UnixNano() > item.Expiration {
			return nil, fmt.Errorf("the item has expired. key: %s", key)
		}
	}

	return &CacheItem{
		Value:      item.Value,
		Created:    item.Created,
		Expiration: item.Expiration,
	}, nil
}

func (c *InMemoryCache) Delete(key string) error {
	c.Lock()

	defer c.Unlock()

	if _, err := c.Get(key); err != nil {
		return err
	}

	delete(c.items, key)

	return nil
}

func (c *InMemoryCache) RenameKey(key string, newKey string) error {
	c.Lock()

	defer c.Unlock()

	// TODO  On the One Hand, Verification Is Not Needed Because It Is Done in the Delete Function
	item, err := c.Get(key)
	if err != nil {
		return err
	}

	if err = c.Delete(key); err != nil {
		return err
	}

	c.Set(newKey, item.Value, time.Duration(item.Expiration-item.Created.UnixNano()))

	return nil
}

func (c *InMemoryCache) CacheSize() CacheSize {
	c.Lock()

	defer c.Unlock()

	cacheWeight := unsafe.Sizeof(c.items)
	cacheLen := len(c.items)

	return CacheSize{
		Len:    cacheLen,
		Weight: cacheWeight,
	}
}

// When Copying An Item, You Can Refer To It By ${Key}Copy
// Example:
// Original Key: exampleCacheItem
// Accessing The Copy After Calling The Function: exampleCacheItemCopy
func (c *InMemoryCache) CopyItem(key string) error {
	item, err := c.Get(key)
	if err != nil {
		return err
	}

	c.Set(fmt.Sprintf("%sCopy", key), item.Value, time.Duration(item.Expiration-item.Created.UnixNano()))

	return nil
}

func (c *InMemoryCache) FlashAll() {
	c.items = make(map[string]CacheItem)
}

func (c *InMemoryCache) SaveFile() (*string, error) {

	fileName := fmt.Sprintf("cacheBackup_%s", time.Now().Format(TimeLayout))

	file, err := os.Create(fileName)
	if err != nil {
		return nil, fmt.Errorf("error creating file: %v", err)
	}

	defer func() {
		file.Close()
	}()

	w := bufio.NewWriter(file)

	itemChanel := make(chan CacheBackupItem, c.CacheSize().Len)
	defer close(itemChanel)

	var wg sync.WaitGroup

	go c.savingDataToFile(context.Background(), &wg, itemChanel, w)

	for _, item := range c.items {

		wg.Add(1)

		itemChanel <- CacheBackupItem{
			Key:        item.Key,
			Value:      item.Value,
			Created:    item.Created.Format(TimeLayout),
			Expiration: time.Duration(item.Expiration - item.Created.UnixNano()).String(),
		}
	}

	wg.Wait()

	if err = w.Flush(); err != nil {
		return nil, fmt.Errorf("error writing file: %v", err)
	}

	return &fileName, nil
}

func (c *InMemoryCache) savingDataToFile(
	ctx context.Context,
	wg *sync.WaitGroup,
	itemStream <-chan CacheBackupItem,
	w *bufio.Writer,
) {
	for {
		select {
		case item, ok := <-itemStream:
			if !ok {
				return
			}

			if _, err := w.Write(
				[]byte(
					fmt.Sprintf(
						"{ Key: %s, Value: %s, Created: %s, Expiration: %s }\r\n",
						item.Key,
						item.Value,
						item.Created,
						item.Expiration,
					),
				),
			); err != nil {
				return
			}

			wg.Done()
		}
	}
}

func (c *InMemoryCache) FileDataToCache(nameLoadFile string) error {
	errCn := make(chan error)

	var wg sync.WaitGroup

	wg.Add(1)

	go func() {

		defer wg.Done()

		err := c.loadFile(nameLoadFile)
		if err != nil {
			errCn <- err
		}
	}()

	wg.Wait()

	select {
	case err := <-errCn:
		return err
	default:
		return nil
	}
}

func (c *InMemoryCache) loadFile(nameLoadFile string) error {
	file, openFileErr := os.Open(nameLoadFile)
	if openFileErr != nil {
		return fmt.Errorf("error open file with name: %s", nameLoadFile)
	}

	scanner := bufio.NewScanner(file)

	backupItems := []CacheBackupItem{}

	for scanner.Scan() {
		line := scanner.Text()
		data, err := extractDataFromLine(line)
		if err != nil {
			return fmt.Errorf("error on LoadFile: %s. ", nameLoadFile)
		}

		backupItems = append(backupItems, data)
	}

	items := []CacheItem{}

	for _, backupItem := range backupItems {
		created, _ := time.Parse(TimeLayout, backupItem.Created)
		expiration, _ := time.ParseDuration(backupItem.Expiration)

		items = append(items, CacheItem{
			Key:        backupItem.Key,
			Value:      backupItem.Value,
			Created:    created,
			Expiration: expiration.Nanoseconds(),
		})
	}

	for _, item := range items {
		c.Set(item.Key, item.Value, time.Duration(item.Expiration))
	}

	return nil
}

func extractDataFromLine(line string) (CacheBackupItem, error) {
	var data CacheBackupItem

	re := regexp.MustCompile(`Key: (.*?), Value: (.*?), Created: (.*?), Expiration: (.*?)}`)
	matches := re.FindStringSubmatch(line)
	if len(matches) != 5 {
		return data, fmt.Errorf("error reading line from file")
	}

	data.Key = strings.TrimSpace(matches[1])
	data.Value = strings.TrimSpace(matches[2])
	data.Created = strings.TrimSpace(matches[3])
	data.Expiration = strings.TrimSpace(matches[4])

	return data, nil
}

func (c *InMemoryCache) cronSavingDataToFile(delay time.Duration) {
	for {
		<-time.After(delay)

		c.SaveFile()
	}
}

func (c *InMemoryCache) gC() {
	for {
		<-time.After(c.cleanupInterval)

		if c.items == nil {
			return
		}

		if keys := c.expiredKeys(); len(keys) != 0 {
			c.clearItems(keys)
		}
	}
}

func (c *InMemoryCache) expiredKeys() (keys []string) {

	c.RLock()

	defer c.RUnlock()

	for k, i := range c.items {
		if time.Now().UnixNano() > i.Expiration && i.Expiration > 0 {
			keys = append(keys, k)
		}
	}

	return
}

func (c *InMemoryCache) clearItems(keys []string) {

	c.Lock()

	defer c.Unlock()

	for _, k := range keys {
		delete(c.items, k)
	}
}

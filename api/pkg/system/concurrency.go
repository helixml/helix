package system

import "sync"

func ForEachConcurrently[ItemType any](items []ItemType, concurrency int, fn func(item ItemType, index int)) {
	semaphore := make(chan struct{}, concurrency)
	var wg sync.WaitGroup

	for index, item := range items {
		wg.Add(1)
		semaphore <- struct{}{} // Block if concurrency is reached
		i := index
		go func(item ItemType) {
			defer wg.Done()
			fn(item, i)
			<-semaphore // Release a slot
		}(item)
	}

	wg.Wait()
}

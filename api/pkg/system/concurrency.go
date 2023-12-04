package system

import (
	"sync"
)

func ForEachConcurrently[ItemType any](
	items []ItemType,
	concurrency int,
	handler func(item ItemType, index int) error,
) error {
	done := make(chan bool)
	errChan := make(chan error)
	go func() {
		semaphore := make(chan struct{}, concurrency)
		var wg sync.WaitGroup
		for index, item := range items {
			wg.Add(1)
			semaphore <- struct{}{} // Block if concurrency is reached
			i := index
			go func(item ItemType) {
				defer wg.Done()
				if e := handler(item, i); e != nil {
					errChan <- e
				}
				<-semaphore // Release a slot
			}(item)
		}
		wg.Wait()
		done <- true
	}()
	select {
	case <-done:
		return nil
	case err := <-errChan:
		return err
	}
}

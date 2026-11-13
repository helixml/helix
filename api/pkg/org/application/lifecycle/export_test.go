package lifecycle

// WaitDeletes blocks until every background delete has finished.
func (s *Service) WaitDeletes() { s.deletes.Wait() }

package team

func (s *Store) loadTasksCurrent(teamID string, limit int) ([]Task, error) {
	if err := s.ensureTaskIndex(teamID); err != nil {
		return nil, err
	}
	return s.loadTasksFromIndex(teamID, limit)
}

func (s *Store) loadTasksCurrentLocked(teamID string, limit int) ([]Task, error) {
	if err := s.ensureTaskIndexLocked(teamID); err != nil {
		return nil, err
	}
	return s.loadTasksFromIndex(teamID, limit)
}

func (s *Store) loadTaskCurrent(teamID, taskID string) (Task, error) {
	if err := s.ensureTaskIndex(teamID); err != nil {
		return Task{}, err
	}
	return s.loadTaskFromIndex(teamID, taskID)
}

func (s *Store) loadTaskCurrentLocked(teamID, taskID string) (Task, error) {
	if err := s.ensureTaskIndexLocked(teamID); err != nil {
		return Task{}, err
	}
	return s.loadTaskFromIndex(teamID, taskID)
}

func (s *Store) appendTaskCurrentLocked(teamID string, task Task) error {
	if err := s.ensureTaskIndexLocked(teamID); err != nil {
		return err
	}
	return s.appendTaskIndexedLocked(teamID, task)
}

func (s *Store) saveTaskCurrentLocked(teamID string, task Task, policy Policy) error {
	if err := s.ensureTaskIndexLocked(teamID); err != nil {
		return err
	}
	return s.saveTaskIndexedLocked(teamID, task, policy)
}

func (s *Store) deleteTaskCurrentLocked(teamID, taskID string) error {
	if err := s.ensureTaskIndexLocked(teamID); err != nil {
		return err
	}
	return s.deleteTaskIndexedLocked(teamID, taskID)
}

func (s *Store) loadArtifactsCurrent(teamID string, limit int) ([]Artifact, error) {
	if err := s.ensureArtifactIndex(teamID); err != nil {
		return nil, err
	}
	return s.loadArtifactsFromIndex(teamID, limit)
}

func (s *Store) loadArtifactsCurrentLocked(teamID string, limit int) ([]Artifact, error) {
	if err := s.ensureArtifactIndexLocked(teamID); err != nil {
		return nil, err
	}
	return s.loadArtifactsFromIndex(teamID, limit)
}

func (s *Store) loadArtifactCurrent(teamID, artifactID string) (Artifact, error) {
	if err := s.ensureArtifactIndex(teamID); err != nil {
		return Artifact{}, err
	}
	return s.loadArtifactFromIndex(teamID, artifactID)
}

func (s *Store) appendArtifactCurrentLocked(teamID string, artifact Artifact) error {
	if err := s.ensureArtifactIndexLocked(teamID); err != nil {
		return err
	}
	return s.appendArtifactIndexedLocked(teamID, artifact)
}

func (s *Store) saveArtifactCurrentLocked(teamID string, artifact Artifact) error {
	if err := s.ensureArtifactIndexLocked(teamID); err != nil {
		return err
	}
	return s.saveArtifactIndexedLocked(teamID, artifact)
}

func (s *Store) deleteArtifactCurrentLocked(teamID, artifactID string) error {
	if err := s.ensureArtifactIndexLocked(teamID); err != nil {
		return err
	}
	return s.deleteArtifactIndexedLocked(teamID, artifactID)
}

func (s *Store) upsertReplicatedTaskCurrentLocked(teamID string, task Task) error {
	if err := s.ensureTaskIndexLocked(teamID); err != nil {
		return err
	}
	return s.appendTaskIndexedLocked(teamID, task)
}

func (s *Store) upsertReplicatedArtifactCurrentLocked(teamID string, artifact Artifact) error {
	if err := s.ensureArtifactIndexLocked(teamID); err != nil {
		return err
	}
	return s.appendArtifactIndexedLocked(teamID, artifact)
}

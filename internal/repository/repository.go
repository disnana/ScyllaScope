package repository

import (
	"scyllascope/internal/model"
	"time"
)

type Repository interface {
	Add(model.Event) error
	Recent(int) ([]model.Event, error)
	Incidents(bool) ([]model.Incident, error)
	PutIncident(model.Incident) error
	WorkIncidents(after string, limit int) ([]model.Incident, error)
	Incident(string) (model.Incident, error)
	UpdateIncidentAction(string, bool, *time.Time) error
	AddSnapshot(model.Snapshot) error
	History(time.Time, int) ([]model.Sample, error)
	NodeHistory(string, time.Time, int) ([]model.Node, error)
	SaveTopology(model.Snapshot) error
	LoadTopology() (model.Snapshot, error)
	Prune(int) error
}

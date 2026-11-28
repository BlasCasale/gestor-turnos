package main

import (
	"testing"
	"time"
)

func TestScheduler(t *testing.T) {

	fechaFija := time.Date(2026, 11, 28, 0, 0, 0, 0, time.UTC)

	generalCorto := &Paciente{EsperandoDesde: fechaFija.Add(-5 * time.Minute), Prioridad: "G"}
	prioritarioCorto := &Paciente{EsperandoDesde: fechaFija, Prioridad: "P"}

	generalLargo := &Paciente{EsperandoDesde: fechaFija.Add(-40 * time.Minute), Prioridad: "G"}
	prioritarioLargo := &Paciente{EsperandoDesde: fechaFija.Add(-2 * time.Minute), Prioridad: "P"}

	generalLimite := &Paciente{EsperandoDesde: fechaFija.Add(-45 * time.Minute), Prioridad: "G"}
	priotarioLimite := &Paciente{EsperandoDesde: fechaFija.Add(-5 * time.Minute), Prioridad: "P"}

	generalExceso := &Paciente{EsperandoDesde: fechaFija.Add(-45 * time.Minute), Prioridad: "G"}
	prioritarioExceso := &Paciente{EsperandoDesde: fechaFija.Add(-5 * time.Minute), Prioridad: "P"}

	casos := []struct {
		nombreCaso  string
		general     *Paciente
		prioritario *Paciente
		ahora       time.Time
		esperado    string
	}{
		{
			nombreCaso:  "Prioritario gana en espera corta",
			general:     generalCorto,
			prioritario: prioritarioCorto,
			ahora:       fechaFija.Add(3 * time.Minute),
			esperado:    prioritarioCorto.Prioridad,
		},
		{
			nombreCaso:  "General gana en espera larga",
			general:     generalLargo,
			prioritario: prioritarioLargo,
			ahora:       fechaFija.Add(3 * time.Minute),
			esperado:    generalLargo.Prioridad,
		},
		{
			nombreCaso:  "Prioritario gana en limite",
			general:     generalLimite,
			prioritario: priotarioLimite,
			ahora:       fechaFija,
			esperado:    generalLimite.Prioridad,
		},
		{
			nombreCaso:  "Prioritario gana en exceso",
			general:     generalExceso,
			prioritario: prioritarioExceso,
			ahora:       fechaFija.Add(1 * time.Minute),
			esperado:    generalExceso.Prioridad,
		},
	}

	for _, caso := range casos {
		t.Run(caso.nombreCaso, func(t *testing.T) {
			got := scheduler(caso.general, caso.prioritario, caso.ahora)
			if got.Prioridad != caso.esperado {
				t.Errorf("esperaba %s, recibi %s", caso.esperado, got.Prioridad)
			}
		})
	}
}

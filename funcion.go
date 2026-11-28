package main

import "time"

type Paciente struct {
	EsperandoDesde time.Time // instante finito de tiempo
	Prioridad      string    // P o G
}

const tiempoMaximo = 45 * time.Minute

func scheduler(general *Paciente, prioritario *Paciente, ahora time.Time) *Paciente {
	if general == nil && prioritario == nil {
		return nil
	}

	if general == nil {
		return prioritario
	}

	if prioritario == nil {
		return general
	}

	espera := ahora.Sub(general.EsperandoDesde)

	if espera >= tiempoMaximo {
		return general
	}

	basePrioritario := 20 * time.Minute

	puntajeGeneral := ahora.Sub(general.EsperandoDesde) * 2
	puntajePrioritario := ahora.Sub(prioritario.EsperandoDesde) + basePrioritario

	if puntajePrioritario >= puntajeGeneral {
		return prioritario
	}
	return general
}

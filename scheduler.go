package main

import (
	"fmt"
	"log"
	"sync"
	"time"
)

type Outcome struct {
	Result    Result
	Triggered bool
	Unchanged bool
	CSVPath   string
	Err       error
}

func execute(q Query, overrides map[string]string, sendNotification bool) Outcome {
	previousState, _ := loadState()
	previous := previousState[q.Name]
	started := time.Now()
	result, err := runQuery(q, overrides)
	outcome := Outcome{Result: result, Err: err}

	current := QueryState{LastRun: started, LastRows: result.Count(), Warnings: result.Warnings}
	if err != nil {
		current.LastError = err.Error()
		if sendNotification {
			notify(appName+": error en "+q.Name, err.Error(), q.Name)
			current.Notified = true
		}
	} else {
		outcome.CSVPath, _ = saveResultCSV(q.Name, result, started)
	}
	triggered, unchanged := evaluate(q, previous, result.Count())
	if err == nil && triggered {
		outcome.Triggered = true
		current.Triggered = true
		if unchanged {
			outcome.Unchanged = true
			current.Seen = previous.Seen
		} else if sendNotification {
			notify(appName+": "+q.Name, fmt.Sprintf("%d registros (regla: %s) en %s", result.Count(), q.RuleText(), q.Connection), q.Name)
			current.Notified = true
		}
	}
	if err := updateState(q.Name, func(s *QueryState) { *s = current }); err != nil {
		log.Printf("no se pudo guardar el estado: %v", err)
	}
	if err := appendHistory(q.Name, countPoint{At: started, Rows: result.Count(), Error: err != nil}); err != nil {
		log.Printf("no se pudo guardar el historial: %v", err)
	}
	return outcome
}

func evaluate(q Query, previous QueryState, count int) (triggered, unchanged bool) {
	triggered = q.Matches(count)
	unchanged = q.OnChange && !previous.LastRun.IsZero() && previous.LastError == "" && previous.LastRows == count
	return triggered, unchanged
}

var runMu sync.Mutex

func executeExclusive(q Query, overrides map[string]string, sendNotification bool) Outcome {
	runMu.Lock()
	defer runMu.Unlock()
	return execute(q, overrides, sendNotification)
}

func logOutcome(q Query, outcome Outcome) {
	switch {
	case outcome.Err != nil:
		log.Printf("%s: error: %v", q.Name, outcome.Err)
	case outcome.Triggered:
		if outcome.Unchanged {
			log.Printf("%s: %d registros, cumple la regla pero la cantidad no cambió, sin notificar (%s)", q.Name, outcome.Result.Count(), outcome.CSVPath)
		} else {
			log.Printf("%s: %d registros, cumple la regla (%s), notificado (%s)", q.Name, outcome.Result.Count(), q.RuleText(), outcome.CSVPath)
		}
	default:
		log.Printf("%s: %d registros, no cumple la regla (%s)", q.Name, outcome.Result.Count(), q.RuleText())
	}
}

func runDue(onRun func(Query, Outcome)) {
	queries, err := loadQueries()
	if err != nil {
		log.Printf("no se pudieron leer las consultas: %v", err)
	}
	state, _ := loadState()
	for _, q := range queries {
		if !due(q, state[q.Name], time.Now()) {
			continue
		}
		outcome := executeExclusive(q, nil, true)
		logOutcome(q, outcome)
		if onRun != nil {
			onRun(q, outcome)
		}
	}
}

func serve() {
	log.Printf("dbwatch serve: revisando consultas cada 30 s en %s", queriesPath())
	for {
		runDue(nil)
		time.Sleep(30 * time.Second)
	}
}

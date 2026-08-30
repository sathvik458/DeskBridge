import { useCallback, useState } from 'react'
import { api } from '../api/client'
import type { Goal } from '../api/types'
import { usePoll } from '../hooks/usePoll'
import { GoalsCard } from '../components/GoalsCard'
import { todayISO } from '../lib'

export function StudyPlanPage() {
  const [date, setDate] = useState(todayISO)

  const fetchGoals = useCallback(() => api.goals(date), [date])
  const goals = usePoll<Goal[]>(fetchGoals, 15000)

  return (
    <div className="stack">
      <div className="row">
        <h1>Study Plan</h1>
        <label className="field">
          <span className="visually-hidden">Date</span>
          <input type="date" value={date} onChange={(event) => setDate(event.target.value)} />
        </label>
      </div>

      <GoalsCard poll={goals} date={date} showDelete />
    </div>
  )
}

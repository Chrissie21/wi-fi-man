import React from 'react'
import ReactDOM from 'react-dom/client'
import './styles.css'

type Role = 'super_admin' | 'cashier' | 'support'

type Plan = {
  id: string
  name: string
  duration_minutes: number
  data_limit_mb: number
  speed_down_kbps: number
  speed_up_kbps: number
  device_limit: number
  price: number
  active: boolean
}

type Sales = {
  paid_count: number
  paid_total: number
}

type CreatePlanForm = {
  name: string
  duration_minutes: number
  data_limit_mb: number
  speed_down_kbps: number
  speed_up_kbps: number
  device_limit: number
  price: number
  active: boolean
}

const initialForm: CreatePlanForm = {
  name: '',
  duration_minutes: 60,
  data_limit_mb: 1024,
  speed_down_kbps: 3000,
  speed_up_kbps: 1000,
  device_limit: 1,
  price: 1,
  active: true
}

function App() {
  const [role, setRole] = React.useState<Role>('super_admin')
  const [plans, setPlans] = React.useState<Plan[]>([])
  const [sales, setSales] = React.useState<Sales | null>(null)
  const [form, setForm] = React.useState<CreatePlanForm>(initialForm)
  const [status, setStatus] = React.useState<string>('')
  const [loading, setLoading] = React.useState<boolean>(false)

  const headers = React.useMemo(() => ({ 'X-Role': role, 'Content-Type': 'application/json' }), [role])

  const loadData = React.useCallback(async () => {
    try {
      const [plansRes, salesRes] = await Promise.all([
        fetch('/v1/admin/plans', { headers }),
        fetch('/v1/admin/reports/sales', { headers })
      ])

      const plansJson = await plansRes.json()
      const salesJson = await salesRes.json()
      setPlans(plansJson.plans ?? [])
      setSales(salesJson)
    } catch {
      setPlans([])
      setSales(null)
    }
  }, [headers])

  React.useEffect(() => {
    loadData()
  }, [loadData])

  async function submitPlan(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (role === 'support') {
      setStatus('Support role cannot create plans.')
      return
    }
    setLoading(true)
    setStatus('')
    try {
      const response = await fetch('/v1/admin/plans', {
        method: 'POST',
        headers,
        body: JSON.stringify(form)
      })
      const data = await response.json()
      if (!response.ok) {
        setStatus(data.error ?? 'Failed to create plan.')
        return
      }
      setStatus(`Plan created: ${data.plan.name}`)
      setForm(initialForm)
      await loadData()
    } catch {
      setStatus('Network error while creating plan.')
    } finally {
      setLoading(false)
    }
  }

  return (
    <main className="layout">
      <header className="header">
        <div>
          <h1>Wi-Fi Admin Console</h1>
          <p>Manage customer packages and monitor sales in real time.</p>
        </div>
        <div className="role-switch">
          <label htmlFor="role">Role</label>
          <select id="role" value={role} onChange={(e) => setRole(e.target.value as Role)}>
            <option value="super_admin">Super Admin</option>
            <option value="cashier">Cashier</option>
            <option value="support">Support</option>
          </select>
        </div>
      </header>

      <section className="panel stats">
        <h2>Sales Snapshot</h2>
        {sales ? (
          <div className="stats-grid">
            <div>
              <strong>{sales.paid_count}</strong>
              <span>Paid Transactions</span>
            </div>
            <div>
              <strong>${sales.paid_total.toFixed(2)}</strong>
              <span>Total Revenue</span>
            </div>
          </div>
        ) : (
          <p>Unable to load sales.</p>
        )}
      </section>

      <section className="panel split">
        <div>
          <h2>Available Packages</h2>
          <p className="sub">These are visible on the customer portal.</p>
          <div className="plan-list">
            {plans.map((plan) => (
              <article className="plan-item" key={plan.id}>
                <h3>{plan.name}</h3>
                <p>
                  {plan.duration_minutes} mins • {plan.data_limit_mb} MB • ${plan.price}
                </p>
                <small>
                  {plan.speed_down_kbps}/{plan.speed_up_kbps} kbps • {plan.device_limit} device(s)
                </small>
              </article>
            ))}
          </div>
        </div>

        <form className="plan-form" onSubmit={submitPlan}>
          <h2>Add Package</h2>
          <p className="sub">Admin/cashier can add new packages for customer purchase.</p>

          <label>
            Name
            <input value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} required />
          </label>
          <label>
            Duration (minutes)
            <input type="number" min={1} value={form.duration_minutes} onChange={(e) => setForm({ ...form, duration_minutes: Number(e.target.value) })} required />
          </label>
          <label>
            Data limit (MB)
            <input type="number" min={0} value={form.data_limit_mb} onChange={(e) => setForm({ ...form, data_limit_mb: Number(e.target.value) })} required />
          </label>
          <div className="two-col">
            <label>
              Down (kbps)
              <input type="number" min={0} value={form.speed_down_kbps} onChange={(e) => setForm({ ...form, speed_down_kbps: Number(e.target.value) })} required />
            </label>
            <label>
              Up (kbps)
              <input type="number" min={0} value={form.speed_up_kbps} onChange={(e) => setForm({ ...form, speed_up_kbps: Number(e.target.value) })} required />
            </label>
          </div>
          <div className="two-col">
            <label>
              Devices
              <input type="number" min={1} value={form.device_limit} onChange={(e) => setForm({ ...form, device_limit: Number(e.target.value) })} required />
            </label>
            <label>
              Price (USD)
              <input type="number" min={0} step="0.01" value={form.price} onChange={(e) => setForm({ ...form, price: Number(e.target.value) })} required />
            </label>
          </div>
          <label className="checkbox">
            <input type="checkbox" checked={form.active} onChange={(e) => setForm({ ...form, active: e.target.checked })} />
            Active package
          </label>

          <button type="submit" disabled={loading || role === 'support'}>
            {loading ? 'Saving...' : 'Create Package'}
          </button>

          {status && <p className="status">{status}</p>}
        </form>
      </section>

      <section className="panel links">
        <h2>Testing Links</h2>
        <a href="/portal" target="_blank" rel="noreferrer">Open Customer Portal</a>
        <a href="/healthz" target="_blank" rel="noreferrer">Health Endpoint</a>
      </section>
    </main>
  )
}

ReactDOM.createRoot(document.getElementById('root')!).render(
  <React.StrictMode>
    <App />
  </React.StrictMode>
)

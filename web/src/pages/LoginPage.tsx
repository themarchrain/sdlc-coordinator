import { Navigate, useLocation } from 'react-router-dom'

export function LoginPage() {
  const location = useLocation()
  const returnTo = new URLSearchParams(location.search).get('return_to')
  const search = returnTo ? `?return_to=${encodeURIComponent(returnTo)}` : ''
  return <Navigate to={`/portal/login${search}`} replace />
}

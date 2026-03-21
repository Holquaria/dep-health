import React from 'react'
import { Routes, Route, NavLink } from 'react-router-dom'
import ScanList from './pages/ScanList.jsx'
import ScanDetail from './pages/ScanDetail.jsx'

export default function App() {
  return (
    <div className="app">
      <header className="header">
        <NavLink to="/" className="logo">
          dep-health
        </NavLink>
        <span className="tagline">Dependency risk dashboard</span>
      </header>
      <main className="main">
        <Routes>
          <Route path="/" element={<ScanList />} />
          <Route path="/scans/:id" element={<ScanDetail />} />
        </Routes>
      </main>
    </div>
  )
}

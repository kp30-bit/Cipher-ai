import React, { useState, useEffect } from 'react';
import { getAnalytics } from '../services/api';
import './Analytics.css';

function Analytics() {
  const [analytics, setAnalytics] = useState(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);

  useEffect(() => {
    // Load analytics immediately when component mounts (homepage load)
    loadAnalytics();
  }, []);

  const loadAnalytics = async () => {
    try {
      setError(null);
      const data = await getAnalytics();
      setAnalytics(data);
    } catch (err) {
      setError(err.message || 'Failed to load analytics');
    } finally {
      setLoading(false);
    }
  };

  // Show loading state only on initial load
  if (loading && !analytics) {
    return (
      <div className="analytics-container">
        <div className="analytics-loading">
          <div className="spinner"></div>
          <p>Loading analytics...</p>
        </div>
      </div>
    );
  }

  // Show error state only if we don't have any data
  if (error && !analytics) {
    return (
      <div className="analytics-container">
        <div className="analytics-error">⚠️ {error}</div>
      </div>
    );
  }

  // Always render the dashboard, even if analytics is null (will show 0s)
  const analyticsData = analytics || {
    total_visits: 0,
    unique_users: 0,
    api_hits: 0,
    endpoint_stats: {}
  };

  return (
    <div className="analytics-container">
      <div className="analytics-grid">
        <div className="analytics-card">
          <div className="analytics-card-header">
            <div className="analytics-card-icon">👥</div>
            <div className="analytics-card-label">Unique Users</div>
          </div>
          <div className="analytics-card-value">
            {analyticsData.unique_users?.toLocaleString() || 0}
          </div>
        </div>
        <div className="analytics-card">
          <div className="analytics-card-header">
            <div className="analytics-card-icon">🌐</div>
            <div className="analytics-card-label">Total Visits</div>
          </div>
          <div className="analytics-card-value">
            {analyticsData.total_visits?.toLocaleString() || 0}
          </div>
        </div>
      </div>
    </div>
  );
}

export default Analytics;


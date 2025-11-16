import React, { useState, useEffect } from 'react';
import './App.css';
import ConcallList from './components/ConcallList';
import SearchBar from './components/SearchBar';
import Analytics from './components/Analytics';
import { fetchConcalls, searchConcalls } from './services/api';

function App() {
  const [concalls, setConcalls] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);
  const [currentPage, setCurrentPage] = useState(1);
  const [totalPages, setTotalPages] = useState(1);
  const [total, setTotal] = useState(0);
  const [isSearchMode, setIsSearchMode] = useState(false);
  const [searchQuery, setSearchQuery] = useState('');

  const loadConcalls = async (page = 1, searchName = '') => {
    setLoading(true);
    setError(null);
    
    try {
      let response;
      if (searchName && searchName.trim() !== '') {
        response = await searchConcalls(searchName, page);
        setIsSearchMode(true);
        setSearchQuery(searchName);
      } else {
        response = await fetchConcalls(page);
        setIsSearchMode(false);
        setSearchQuery('');
      }
      
      setConcalls(response.data || []);
      setCurrentPage(response.meta?.page || 1);
      setTotalPages(response.meta?.totalPages || 1);
      setTotal(response.meta?.total || 0);
    } catch (err) {
      setError(err.message || 'Failed to load concalls');
      setConcalls([]);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    loadConcalls(1);
  }, []);

  const handlePageChange = (newPage) => {
    if (newPage >= 1 && newPage <= totalPages) {
      if (isSearchMode && searchQuery) {
        loadConcalls(newPage, searchQuery);
      } else {
        loadConcalls(newPage);
      }
      window.scrollTo({ top: 0, behavior: 'smooth' });
    }
  };

  const handleSearch = (query) => {
    if (query && query.trim() !== '') {
      loadConcalls(1, query);
    } else {
      loadConcalls(1);
    }
  };

  const handleClearSearch = () => {
    setSearchQuery('');
    setIsSearchMode(false);
    loadConcalls(1);
  };

  return (
    <div className="App">
      <div className="container">
        <header className="header">
          <h1 className="title">📊 Concall Analyser</h1>
          <p className="subtitle">Earnings Call Transcripts & Guidance</p>
        </header>

        <SearchBar 
          onSearch={handleSearch}
          onClear={handleClearSearch}
          isSearchMode={isSearchMode}
          searchQuery={searchQuery}
        />

        {error && (
          <div className="error-message">
            <span>⚠️</span> {error}
          </div>
        )}

        <ConcallList 
          concalls={concalls}
          loading={loading}
          isSearchMode={isSearchMode}
          searchQuery={searchQuery}
          total={total}
        />

        {!loading && concalls.length > 0 && (
          <div className="pagination-container">
            <div className="pagination-info">
              Page {currentPage} of {totalPages} ({total} total)
            </div>
            <div className="pagination-buttons">
              <button
                className="pagination-btn"
                onClick={() => handlePageChange(currentPage - 1)}
                disabled={currentPage === 1}
              >
                ← Previous
              </button>
              <button
                className="pagination-btn"
                onClick={() => handlePageChange(currentPage + 1)}
                disabled={currentPage === totalPages}
              >
                Next →
              </button>
            </div>
          </div>
        )}

        <Analytics />
      </div>
    </div>
  );
}

export default App;


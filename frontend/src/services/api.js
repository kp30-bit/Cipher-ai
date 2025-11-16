const API_BASE_URL = process.env.REACT_APP_API_URL || '/api';

const handleResponse = async (response) => {
  if (!response.ok) {
    const error = await response.json().catch(() => ({ error: 'Network error' }));
    throw new Error(error.error || `HTTP error! status: ${response.status}`);
  }
  return response.json();
};

export const fetchConcalls = async (page = 1, limit = 12) => {
  const url = `${API_BASE_URL}/list_concalls?page=${page}&limit=${limit}`;
  const response = await fetch(url);
  return handleResponse(response);
};

export const searchConcalls = async (name, page = 1, limit = 12) => {
  const encodedName = encodeURIComponent(name);
  const url = `${API_BASE_URL}/find_concalls?name=${encodedName}&page=${page}&limit=${limit}`;
  const response = await fetch(url);
  return handleResponse(response);
};


import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { useEffect } from 'react';
import { Link, MemoryRouter, Outlet, Route, Routes, useLocation } from 'react-router-dom';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { RouteErrorBoundary } from './RouteErrorBoundary';

const Broken = (): never => {
  throw new Error('page exploded');
};

afterEach(cleanup);

describe('RouteErrorBoundary', () => {
  it('replaces only the failing page with the error card', () => {
    const quiet = vi.spyOn(console, 'error').mockImplementation(() => {});
    render(
      <>
        <header>navigation</header>
        <RouteErrorBoundary>
          <Broken />
        </RouteErrorBoundary>
      </>,
    );
    quiet.mockRestore();
    expect(screen.getByText('This page hit an error')).toBeTruthy();
    expect(screen.getByText('page exploded')).toBeTruthy();
    expect(screen.getByText('navigation')).toBeTruthy();
  });

  it('renders a page that does not throw as it is', () => {
    render(
      <RouteErrorBoundary>
        <p>wallet</p>
      </RouteErrorBoundary>,
    );
    expect(screen.getByText('wallet')).toBeTruthy();
    expect(screen.queryByText('This page hit an error')).toBeNull();
  });

  it('clears the error card when the reset key changes', () => {
    const quiet = vi.spyOn(console, 'error').mockImplementation(() => {});
    const { rerender } = render(
      <RouteErrorBoundary resetKey="/a">
        <Broken />
      </RouteErrorBoundary>,
    );
    quiet.mockRestore();
    rerender(
      <RouteErrorBoundary resetKey="/a">
        <p>wallet</p>
      </RouteErrorBoundary>,
    );
    expect(screen.getByText('This page hit an error')).toBeTruthy();
    rerender(
      <RouteErrorBoundary resetKey="/b">
        <p>wallet</p>
      </RouteErrorBoundary>,
    );
    expect(screen.queryByText('This page hit an error')).toBeNull();
    expect(screen.getByText('wallet')).toBeTruthy();
  });

  it('keeps a section layout mounted while its pages change', () => {
    let mounts = 0;
    const Layout = () => {
      useEffect(() => {
        mounts++;
      }, []);
      return (
        <>
          <Link to="/s/b">to b</Link>
          <Outlet />
        </>
      );
    };
    const Shell = () => {
      const location = useLocation();
      return (
        <RouteErrorBoundary resetKey={location.pathname}>
          <Routes>
            <Route path="/s" element={<Layout />}>
              <Route path="a" element={<p>page a</p>} />
              <Route path="b" element={<p>page b</p>} />
            </Route>
          </Routes>
        </RouteErrorBoundary>
      );
    };
    render(
      <MemoryRouter initialEntries={['/s/a']}>
        <Shell />
      </MemoryRouter>,
    );
    fireEvent.click(screen.getByText('to b'));
    expect(screen.getByText('page b')).toBeTruthy();
    expect(mounts).toBe(1);
  });
});

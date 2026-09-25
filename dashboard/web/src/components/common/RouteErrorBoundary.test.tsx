import { cleanup, render, screen } from '@testing-library/react';
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
});

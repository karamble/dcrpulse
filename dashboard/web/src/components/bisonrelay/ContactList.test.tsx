import { act, cleanup, fireEvent, render, renderHook, screen } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import type { BisonrelayContact } from '../../services/bisonrelayApi';
import { ContactList, filterContacts, useUidSelection } from './ContactList';

vi.mock('../../services/bisonrelayApi', () => ({ getBisonrelayContacts: vi.fn() }));

afterEach(cleanup);

const contact = (identity: string | undefined, nick: string) =>
  ({ id: identity === undefined ? undefined : { identity, nick } }) as unknown as BisonrelayContact;
const alice = contact('a1a1a1a1a1a1', 'Alice');
const bob = contact('b2b2b2b2b2b2', 'bob');

describe('filterContacts', () => {
  it('matches the nick ignoring case and drops contacts without an identity', () => {
    expect(filterContacts([alice, bob, contact(undefined, 'ghost')], 'ALI')).toEqual([alice]);
    expect(filterContacts([alice, bob, contact(undefined, 'ghost')])).toEqual([alice, bob]);
  });
});

describe('useUidSelection', () => {
  it('toggles, and adds only below the limit', () => {
    const { result } = renderHook(() => useUidSelection());
    act(() => result.current.toggle('x', 1));
    act(() => result.current.toggle('y', 1));
    expect([...result.current.selected]).toEqual(['x']);
    act(() => result.current.toggle('x'));
    expect(result.current.selected.size).toBe(0);
  });
});

describe('ContactList', () => {
  it('shows checkboxes only for a multi-select and keeps disabled rows unpickable', () => {
    const onPick = vi.fn();
    const { container, rerender } = render(<ContactList contacts={[alice, bob]} emptyText="none" onPick={onPick} />);
    expect(container.querySelectorAll('span.rounded.border').length).toBe(0);
    rerender(
      <ContactList
        contacts={[alice, bob]}
        emptyText="none"
        onPick={onPick}
        selected={new Set(['a1a1a1a1a1a1'])}
        isDisabled={(uid) => uid === 'b2b2b2b2b2b2'}
      />,
    );
    expect(container.querySelectorAll('span.rounded.border').length).toBe(2);
    fireEvent.click(screen.getByText('bob'));
    expect(onPick).not.toHaveBeenCalled();
    fireEvent.click(screen.getByText('Alice'));
    expect(onPick).toHaveBeenCalledWith(alice);
  });

  it('shows the loading and empty states', () => {
    const { rerender } = render(<ContactList contacts={null} emptyText="none" onPick={() => {}} />);
    expect(screen.getByText('Loading contacts…')).toBeTruthy();
    rerender(<ContactList contacts={[]} emptyText="nobody here" onPick={() => {}} />);
    expect(screen.getByText('nobody here')).toBeTruthy();
  });
});

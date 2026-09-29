import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { Modal } from './Modal';

afterEach(cleanup);

const open = (props: { busy?: boolean; onSubmit?: () => void } = {}) => {
  const onClose = vi.fn();
  const view = render(
    <div data-testid="page">
      <Modal onClose={onClose} busy={props.busy} as={props.onSubmit ? 'form' : 'div'} onSubmit={props.onSubmit}>
        <button type="submit">inside</button>
      </Modal>
    </div>,
  );
  return { onClose, view };
};

describe('Modal', () => {
  it('renders into the body, outside the page that opened it', () => {
    const { view } = open();
    const inside = screen.getByText('inside');
    expect(view.getByTestId('page').contains(inside)).toBe(false);
    expect(document.body.contains(inside)).toBe(true);
  });

  it('closes on Escape and on a backdrop click, not on a click inside', () => {
    const { onClose } = open();
    fireEvent.click(screen.getByText('inside'));
    expect(onClose).not.toHaveBeenCalled();
    fireEvent.keyDown(window, { key: 'Escape' });
    expect(onClose).toHaveBeenCalledTimes(1);
    fireEvent.click(screen.getByText('inside').parentElement!.parentElement!);
    expect(onClose).toHaveBeenCalledTimes(2);
  });

  it('stays open while busy', () => {
    const { onClose } = open({ busy: true });
    fireEvent.keyDown(window, { key: 'Escape' });
    fireEvent.click(screen.getByText('inside').parentElement!.parentElement!);
    expect(onClose).not.toHaveBeenCalled();
  });

  it('submits when it is the form', () => {
    const onSubmit = vi.fn((e?: { preventDefault?: () => void }) => e?.preventDefault?.());
    open({ onSubmit });
    fireEvent.click(screen.getByText('inside'));
    expect(onSubmit).toHaveBeenCalledTimes(1);
  });
});

// Site-styled replacement for the browser's native confirm()/alert() dialogs.
// A single <ConfirmDialog /> is mounted in each root layout and reads this
// singleton's state. Callers use the imperative API:
//
//   if (!(await confirmer.ask({ title: 'Delete node?', message: '…', danger: true }))) return;
//   await confirmer.tell({ title: 'Failed', message: err.message });
//
// `ask` resolves true/false when the user confirms/cancels; `tell` resolves
// when the single OK button is pressed. Both close on Escape / backdrop click
// (treated as cancel / dismiss).

export type ConfirmOpts = {
	title: string;
	message?: string;
	/** Confirm button label. Default "Confirm". */
	confirmLabel?: string;
	/** Cancel button label. Default "Cancel". */
	cancelLabel?: string;
	/** Style the confirm button as a destructive action. */
	danger?: boolean;
	/** Monospace detail shown on its own line below the message — e.g. a public
	 *  key. Rendered small so a full 64-char key fits one line; scrolls if longer. */
	code?: string;
};

type State = ConfirmOpts & {
	open: boolean;
	/** When true, only an OK button is shown (alert-style). */
	notice: boolean;
	resolve: ((ok: boolean) => void) | null;
};

class Confirmer {
	state = $state<State>({ open: false, notice: false, title: '', resolve: null });

	/** Ask for confirmation. Resolves true if confirmed, false if cancelled. */
	ask(opts: ConfirmOpts): Promise<boolean> {
		return new Promise((resolve) => {
			this.settle(false); // clear any dialog already open
			this.state = { ...opts, open: true, notice: false, resolve };
		});
	}

	/** Show a dismissible notice (alert replacement). Resolves when dismissed. */
	tell(opts: ConfirmOpts): Promise<boolean> {
		return new Promise((resolve) => {
			this.settle(false);
			this.state = { ...opts, open: true, notice: true, resolve };
		});
	}

	/** Resolve the pending promise (if any) and close. */
	private settle(ok: boolean) {
		const r = this.state.resolve;
		this.state = { open: false, notice: false, title: '', resolve: null };
		r?.(ok);
	}

	confirm() {
		this.settle(true);
	}

	cancel() {
		this.settle(false);
	}
}

export const confirmer = new Confirmer();

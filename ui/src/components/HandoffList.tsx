import type { Order } from "~/client";

function isReadyForHandoff(
  order: Order,
): order is Order & { branch_name: string; base_branch: string } {
  return order.status === "completed" && Boolean(order.branch_name && order.base_branch);
}

function safeForShell(ref: string) {
  return /^[A-Za-z0-9._/-]+$/.test(ref);
}

export function HandoffList({ orders }: { orders: Order[] }) {
  const ready = orders.filter(isReadyForHandoff);
  if (ready.length === 0) {
    return null;
  }

  return (
    <section aria-label="Ready for handoff" className="border-b border-border-subtle px-6 py-4">
      <h2 className="font-display text-sm font-bold text-accent uppercase tracking-wider mb-3">
        Ready for handoff
      </h2>
      <p className="text-sm text-text-secondary mb-3">
        Noodle has finished these orders. Publish each branch from the Noodle clone when ready.
      </p>
      <div className="grid gap-3">
        {ready.map((order) => (
          <div key={order.id} className="border border-border-subtle bg-bg-surface p-3">
            <div className="font-display font-bold text-text-primary">
              {order.title || order.id}
            </div>
            <div className="text-xs text-text-secondary mt-1">
              Branch <code className="text-text-primary">{order.branch_name}</code> · Base{" "}
              <code className="text-text-primary">{order.base_branch}</code>
            </div>
            {safeForShell(order.branch_name) && safeForShell(order.base_branch) ? (
              <>
                <div className="mt-2 text-xs text-text-secondary">
                  From the Noodle clone, push the branch and open a PR:
                </div>
                <pre className="mt-1 overflow-x-auto text-xs text-text-primary">
                  <code>{`git push -u origin ${order.branch_name}\ngh pr create --base ${order.base_branch} --head ${order.branch_name}`}</code>
                </pre>
              </>
            ) : (
              <p className="mt-2 text-xs text-text-secondary">
                Push this branch from the Noodle clone, then open a PR against the base branch.
                Quote the branch names for your shell.
              </p>
            )}
          </div>
        ))}
      </div>
    </section>
  );
}

// The checkbox square of the app's pickers (category, tag and column
// choosers): a filled slate square with a white check when on, a bordered
// white square when off. Purely visual; the row it sits in is the control.

import { CheckIcon } from "lucide-react";

export function CheckSquare({ checked }: { checked: boolean }) {
    return (
        <span
            className={`size-3.5 rounded border flex items-center justify-center transition-colors shrink-0 ${
                checked ? "border-slate-900 bg-slate-900" : "border-slate-300 bg-white"
            }`}
        >
            {checked && <CheckIcon className="w-2 h-2 text-white" />}
        </span>
    );
}


interface SectionPageProps {
  title: string
  description: string
  nextPhase: string
}

export function SectionPage({
  title,
  description,
  nextPhase,
}: SectionPageProps) {
  return (
    <div className="space-y-7">
      <div>
        <p className="text-sm font-semibold text-indigo-600">
          Workspace / {title}
        </p>

        <h1 className="mt-2 text-3xl font-bold tracking-tight text-slate-950">
          {title}
        </h1>

        <p className="mt-3 max-w-2xl text-sm leading-7 text-slate-500">
          {description}
        </p>
      </div>

      <div className="rounded-2xl border border-dashed border-slate-300 bg-white px-6 py-16 text-center shadow-sm">
        <div className="mx-auto flex h-14 w-14 items-center justify-center rounded-2xl bg-indigo-50 text-2xl font-bold text-indigo-600">
          +
        </div>

        <h2 className="mt-5 text-xl font-semibold text-slate-900">
          Module foundation ready
        </h2>

        <p className="mx-auto mt-3 max-w-md text-sm leading-7 text-slate-500">
          Navigasi halaman sudah aktif. Komponen, data, dan
          aksi bisnis modul ini akan diimplementasikan pada
          checkpoint frontend berikutnya.
        </p>

        <span className="mt-6 inline-flex rounded-full bg-slate-100 px-4 py-2 text-xs font-semibold text-slate-600">
          Planned for {nextPhase}
        </span>
      </div>
    </div>
  )
}

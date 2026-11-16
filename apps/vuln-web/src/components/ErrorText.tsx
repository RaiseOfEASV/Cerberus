export default function ErrorText({ error }: { error: string }) {
  return error ? <p className="error" role="alert">{error}</p> : null
}

export function message(e: unknown) {
  return e instanceof Error ? e.message : 'Something went wrong.'
}

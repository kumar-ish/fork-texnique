const pickRandom = <T>(list: Array<T>) => {
  return list[Math.floor(Math.random() * list.length)];
};

interface Serializable {
  serialize(): Uint8Array;
}
const makeProtoRequest = async (
  path: string,
  req: Serializable,
  method: string = "post"
) => {
  return await fetch(`http://localhost:8080${path}`, {
    method,
    body: req.serialize(),
    mode: "cors",
  });
};

const readProtoResponse = async (res: Response) => {
  return new Uint8Array(await (await res.blob()).arrayBuffer());
};

export { makeProtoRequest, readProtoResponse, pickRandom };

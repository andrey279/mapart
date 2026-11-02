import java.nio.file.*;
import java.util.*;
import com.google.gson.*;
import net.minecraft.SharedConstants;
import net.minecraft.server.Bootstrap;
import net.minecraft.core.BlockPos;
import net.minecraft.core.registries.BuiltInRegistries;
import net.minecraft.world.level.EmptyBlockGetter;
import net.minecraft.world.level.block.*;
import net.minecraft.world.level.block.state.*;
import net.minecraft.world.level.block.state.properties.*;
import net.minecraft.world.level.material.MapColor;

public class InspectBlocks {
 @SuppressWarnings({"unchecked","rawtypes"})
 static BlockState with(BlockState s, String name, String value) {
  Property p = s.getBlock().getStateDefinition().getProperty(name);
  if (p != null) {
   Optional v = p.getValue(value);
   if (v.isPresent()) return (BlockState)s.setValue(p,(Comparable)v.get());
  }
  return s;
 }
 @SuppressWarnings({"unchecked","rawtypes"})
 static JsonObject inspect(Block b, BlockState s) {
  JsonObject row = new JsonObject();
  row.addProperty("name",BuiltInRegistries.BLOCK.getKey(b).getPath());
  row.addProperty("label_key",b.getDescriptionId());
  MapColor c = s.getMapColor(EmptyBlockGetter.INSTANCE,BlockPos.ZERO);
  row.addProperty("rgb",c.col);row.addProperty("map_color",c.id);
  row.addProperty("falling", b instanceof FallingBlock);
  row.addProperty("solid",s.isSolidRender());
  row.addProperty("class",b.getClass().getSimpleName());
  row.addProperty("item",BuiltInRegistries.ITEM.getKey(b.asItem()).getPath());
  JsonObject properties = new JsonObject();
  for (Property p:s.getProperties()) properties.addProperty(p.getName(),p.getName((Comparable)s.getValue(p)));
  row.add("properties",properties);
  return row;
 }
 public static void main(String[] args) throws Exception {
  SharedConstants.tryDetectVersion();Bootstrap.bootStrap();
  JsonArray result = new JsonArray();
  for(Block b:BuiltInRegistries.BLOCK) {
   BlockState s=b.defaultBlockState();
   s=with(s,"type","top");s=with(s,"half","top");s=with(s,"persistent","true");s=with(s,"waterlogged","false");
   if(b instanceof MultifaceBlock) s=with(s,"down","true");
   result.add(inspect(b,s));
   Property<?> axis=b.getStateDefinition().getProperty("axis");
   if(axis!=null) {
    BlockState side=with(s,"axis","x");
    if(side.getMapColor(EmptyBlockGetter.INSTANCE,BlockPos.ZERO)!=s.getMapColor(EmptyBlockGetter.INSTANCE,BlockPos.ZERO))result.add(inspect(b,side));
   }
  }
  Files.writeString(Path.of(args[0]),new GsonBuilder().setPrettyPrinting().create().toJson(result));
 }
}
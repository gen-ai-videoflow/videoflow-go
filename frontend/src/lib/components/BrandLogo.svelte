<script lang="ts">
 let {compact=false}:{compact?:boolean}=$props();
</script>
<span class="brand-logo" class:compact>
 <img class="logo-dark" src="/brand/videoflow-logo.svg" alt="VideoFlow" width="217" height="64"/>
 <img class="logo-light" src="/brand/videoflow-logo-light.svg" alt="VideoFlow" width="217" height="64"/>
</span>
<style>.brand-logo{display:inline-flex;align-items:center;width:122px;flex-shrink:0;vertical-align:middle}.brand-logo.compact{width:108px}.brand-logo img{width:100%;height:auto;display:block}.brand-logo .logo-dark{display:none}:global(.dark) .brand-logo .logo-dark{display:block}:global(.dark) .brand-logo .logo-light{display:none}</style>
